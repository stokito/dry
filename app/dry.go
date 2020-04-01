package app

import (
	"context"
	"fmt"
	"image"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/events"
	gtermui "github.com/gizak/termui"
	"github.com/moncho/dry/appui"
	"github.com/moncho/dry/appui/swarm"
	docker "github.com/moncho/dry/docker"
	"github.com/moncho/dry/ui"
	"github.com/moncho/dry/ui/termui"
)

//Dry resources and state
type Dry struct {
	dockerDaemon     docker.ContainerDaemon
	dockerEvents     <-chan events.Message
	dockerEventsDone chan<- struct{}
	output           chan string
	showHeader       bool

	sync.RWMutex
	view   viewMode
	paused bool
	screen *ui.Screen
}

func (d *Dry) showingHeader() bool {
	return d.showHeader
}

func (d *Dry) toggleShowHeader() {
	d.showHeader = !d.showHeader
}

//Close closes dry, releasing any resources held by it
func (d *Dry) Close() {
	d.dockerDaemon.Close()
	d.screen.Close()
	close(d.dockerEventsDone)
	close(d.output)
}

//OuputChannel returns the channel where dry messages are written
func (d *Dry) OuputChannel() <-chan string {
	return d.output
}

//changeView changes the active view mode
func (d *Dry) changeView(v viewMode) {
	d.Lock()
	defer d.Unlock()

	d.view = v
}

func (d *Dry) showDockerEvents() {
	go func() {
		for event := range d.dockerEvents {
			//exec_ messages are sent continuously if docker is checking
			//a container's health, so they are ignored
			if strings.Contains(event.Action, "exec_") {
				continue
			}
			//top messages are sent continuously on monitor mode, ignored
			if strings.Contains(event.Action, "top") {
				continue
			}
			d.message(fmt.Sprintf("Docker: %s %s", event.Action, event.ID))
		}
	}()
}

//message publishes the given message
func (d *Dry) message(message string) {
	select {
	case d.output <- message:
	default:
	}
}

func (d *Dry) actionMessage(cid interface{}, action string) {
	d.message(fmt.Sprintf("<red>%s container with id </><white>%v</>",
		action, cid))
}

func (d *Dry) errorMessage(cid interface{}, action string, err error) {
	d.message(err.Error())
}

func (d *Dry) viewMode() viewMode {
	d.RLock()
	defer d.RUnlock()
	return d.view
}

func (d *Dry) isPaused() bool {
	d.RLock()
	defer d.RUnlock()
	return d.paused
}

func (d *Dry) pause() {
	d.Lock()
	defer d.Unlock()
	d.paused = true
}
func (d *Dry) resume() {
	d.Lock()
	defer d.Unlock()
	d.paused = false
}

func (d *Dry) gscreen() *ui.Screen {
	d.RLock()
	defer d.RUnlock()
	return d.screen
}

func (d *Dry) sscreen(s *ui.Screen) {
	d.Lock()
	defer d.Unlock()
	d.screen = s
}

//initRegistry creates a widget registry with its widget ready to be used
func initRegistry(dry *Dry) *widgetRegistry {
	daemon := dry.dockerDaemon
	d := dry.screen.Dimensions()
	height, width := d.Height, d.Width
	di := appui.NewDockerInfo(daemon)
	di.SetX(0)
	di.SetY(1)
	di.SetWidth(width)
	widgetScreen := &screen{dry}
	w := widgetRegistry{
		DockerInfo:    di,
		ContainerList: appui.NewContainersWidget(daemon, widgetScreen),
		ContainerMenu: appui.NewContainerMenuWidget(daemon, widgetScreen),
		ImageList:     appui.NewDockerImagesWidget(daemon.Images, widgetScreen),
		DiskUsage:     appui.NewDockerDiskUsageRenderer(height),
		Monitor:       appui.NewMonitor(daemon, widgetScreen),
		Networks:      appui.NewDockerNetworksWidget(daemon, widgetScreen),
		Nodes:         swarm.NewNodesWidget(daemon, widgetScreen),
		NodeTasks:     swarm.NewNodeTasksWidget(daemon, widgetScreen),
		ServiceTasks:  swarm.NewServiceTasksWidget(daemon, widgetScreen),
		ServiceList:   swarm.NewServicesWidget(daemon, widgetScreen),
		Stacks:        swarm.NewStacksWidget(daemon, widgetScreen),
		StackTasks:    swarm.NewStacksTasksWidget(daemon, widgetScreen),
		widgets:       make(map[string]termui.Widget),
		MessageBar:    ui.NewExpiringMessageWidget(0, dry.screen.RenderAtColumn),
		Volumes:       appui.NewVolumesWidget(daemon, widgetScreen),
	}

	refreshOnContainerEvent(w.ContainerList, daemon)
	refreshOnDockerEvent(docker.ImageSource, w.ImageList, Images)
	refreshOnDockerEvent(docker.NetworkSource, w.Networks, Networks)
	refreshOnDockerEvent(docker.NodeSource, w.Nodes, Nodes)
	refreshOnDockerEvent(docker.ServiceSource, w.ServiceList, Services)
	refreshOnDockerEvent(docker.ServiceSource, w.Stacks, Stacks)
	refreshOnDockerEvent(docker.VolumeSource, w.Volumes, Volumes)

	return &w
}

func newDry(screen *ui.Screen, d docker.ContainerDaemon) (*Dry, error) {
	dockerEvents, dockerEventsDone, err := d.Events()
	if err != nil {
		return nil, err
	}

	dry := &Dry{}
	dry.showHeader = true
	dry.dockerDaemon = d
	dry.output = make(chan string)
	dry.dockerEvents = dockerEvents
	dry.dockerEventsDone = dockerEventsDone
	dry.screen = screen

	widgets = initRegistry(dry)
	viewsToHandlers = initHandlers(dry, screen)
	dry.showDockerEvents()
	return dry, nil

}

//NewDry creates a new dry application
func NewDry(screen *ui.Screen, cfg Config) (*Dry, error) {

	d, err := docker.ConnectToDaemon(cfg.dockerEnv())
	if err != nil {
		return nil, err
	}
	dry, err := newDry(screen, d)
	if err != nil {
		return nil, err
	}
	if cfg.MonitorMode {
		dry.changeView(Monitor)
		widgets.Monitor.RefreshRate(cfg.MonitorRefreshRate)
	}
	return dry, nil
}

var refreshInterval = 250 * time.Millisecond // time to wait before next refresh

func refreshOnDockerEvent(source docker.SourceType, w termui.Widget, view viewMode) {
	last := time.Now()
	var lock sync.Mutex
	docker.GlobalRegistry.Register(
		source,
		func(ctx context.Context, m events.Message) error {
			lock.Lock()
			defer lock.Unlock()
			if time.Since(last) < refreshInterval {
				return nil
			}
			last = time.Now()
			err := w.Unmount()
			if err != nil {
				return err
			}
			return refreshIfView(view)
		})
}
func refreshOnContainerEvent(w termui.Widget, daemon docker.ContainerDaemon) {
	last := time.Now()
	var lock sync.Mutex
	docker.GlobalRegistry.Register(
		docker.ContainerSource,
		func(ctx context.Context, m events.Message) error {
			lock.Lock()
			defer lock.Unlock()
			if time.Since(last) < refreshInterval {
				return nil
			}
			last = time.Now()
			daemon.Refresh(func(e error) {
				err := w.Unmount()
				if err != nil {
					return
				}

				refreshIfView(Main)
			})
			return nil
		})
}

// screen for widgets.
type screen struct {
	dry *Dry
}

func (s *screen) Bounds() image.Rectangle {
	dim := s.dry.gscreen().Dimensions()
	y := 0
	if s.dry.showingHeader() {
		y = appui.MainScreenHeaderSize
	}

	return image.Rect(0, y, dim.Width, dim.Height-appui.MainScreenFooterLength)
}

func (s *screen) Cursor() *ui.Cursor {
	return s.dry.gscreen().Cursor()
}

func (s *screen) Flush() {
	s.dry.gscreen().Flush()
}

func (s *screen) RenderBufferer(bs ...gtermui.Bufferer) {
	s.dry.gscreen().RenderBufferer(bs...)
}
