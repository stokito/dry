package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/moncho/dry/appui"
	"github.com/moncho/dry/docker"
	"github.com/moncho/dry/ui"
	termbox "github.com/nsf/termbox-go"
)

type cMenuEventHandler struct {
	baseEventHandler
}

func (h *cMenuEventHandler) handle(event termbox.Event, f func(eventHandler)) {

	handled := true
	switch event.Key {

	case termbox.KeyEsc:
		h.screen.Cursor.Reset()
		h.dry.SetViewMode(Main)
		f(viewsToHandlers[Main])

	case termbox.KeyEnter:
		err := widgets.ContainerMenu.OnEvent(func(s string) error {
			//s is a string made of two parts: an Id and a description
			//separated by ":"
			cd := strings.Split(s, ":")
			if len(cd) != 2 {
				return errors.New("Invalid command description: " + s)
			}
			id := cd[0]
			command, err := docker.CommandFromDescription(cd[1])
			if err != nil {
				return err
			}
			h.handleCommand(id, command, f)
			return nil
		})
		if err != nil {
			h.dry.appmessage(fmt.Sprintf("Could not run command: %s", err.Error()))
		}
	default:
		handled = false
	}

	if !handled {
		h.baseEventHandler.handle(event, f)
	} else {
		refreshScreen()
	}
}

func (h *cMenuEventHandler) handleCommand(id string, command docker.Command, f func(eventHandler)) {

	dry := h.dry
	screen := h.screen

	container := dry.dockerDaemon.ContainerByID(id)
	switch command {
	case docker.KILL:
		prompt := appui.NewPrompt(
			fmt.Sprintf("Do you want to kill container %s? (y/N)", id))
		widgets.add(prompt)
		forwarder := newEventForwarder()
		f(forwarder)
		refreshScreen()

		go func() {
			events := ui.EventSource{
				Events: forwarder.events(),
				EventHandledCallback: func(e termbox.Event) error {
					return refreshScreen()
				},
			}
			prompt.OnFocus(events)
			conf, cancel := prompt.Text()
			f(h)
			widgets.remove(prompt)
			if cancel || (conf != "y" && conf != "Y") {
				return
			}

			dry.actionMessage(id, "Killing")
			err := dry.dockerDaemon.Kill(id)
			if err == nil {
				widgets.ContainerMenu.ForContainer(id)
				refreshScreen()
			} else {
				dry.errorMessage(id, "killing", err)
			}

		}()
	case docker.RESTART:

		prompt := appui.NewPrompt(
			fmt.Sprintf("Do you want to restart container %s? (y/N)", id))
		widgets.add(prompt)
		forwarder := newEventForwarder()
		f(forwarder)
		refreshScreen()

		go func() {
			events := ui.EventSource{
				Events: forwarder.events(),
				EventHandledCallback: func(e termbox.Event) error {
					return refreshScreen()
				},
			}
			prompt.OnFocus(events)
			conf, cancel := prompt.Text()
			f(h)
			widgets.remove(prompt)
			if cancel || (conf != "y" && conf != "Y") {

				return
			}

			if err := dry.dockerDaemon.RestartContainer(id); err == nil {
				widgets.ContainerMenu.ForContainer(id)
				refreshScreen()
			} else {
				dry.appmessage(
					fmt.Sprintf("Error restarting container %s, err: %s", id, err.Error()))
			}

		}()

	case docker.STOP:

		prompt := appui.NewPrompt(
			fmt.Sprintf("Do you want to stop container %s? (y/N)", id))
		widgets.add(prompt)
		forwarder := newEventForwarder()
		f(forwarder)
		refreshScreen()

		go func() {
			events := ui.EventSource{
				Events: forwarder.events(),
				EventHandledCallback: func(e termbox.Event) error {
					return refreshScreen()
				},
			}
			prompt.OnFocus(events)
			conf, cancel := prompt.Text()
			f(h)
			widgets.remove(prompt)
			if cancel || (conf != "y" && conf != "Y") {

				return
			}

			dry.actionMessage(id, "Stopping")
			err := dry.dockerDaemon.StopContainer(id)
			if err == nil {
				widgets.ContainerMenu.ForContainer(id)
				refreshScreen()
			} else {
				dry.errorMessage(id, "stopping", err)
			}

		}()
	case docker.LOGS:

		prompt := logsPrompt()
		widgets.add(prompt)
		forwarder := newEventForwarder()
		f(forwarder)
		refreshScreen()
		go func() {
			events := ui.EventSource{
				Events: forwarder.events(),
				EventHandledCallback: func(e termbox.Event) error {
					return refreshScreen()
				},
			}
			prompt.OnFocus(events)
			widgets.remove(prompt)
			since, canceled := prompt.Text()

			if canceled {
				return
			}

			logs, err := h.dry.dockerDaemon.Logs(id, since)
			if err == nil {
				appui.Stream(logs, forwarder.events(),
					func() {
						h.dry.SetViewMode(ContainerMenu)
						f(h)
						refreshScreen()
					})
			} else {
				h.dry.appmessage("Error showing container logs: " + err.Error())
			}
		}()
	case docker.RM:
		prompt := appui.NewPrompt(
			fmt.Sprintf("Do you want to remove container %s? (y/N)", id))
		widgets.add(prompt)
		forwarder := newEventForwarder()
		f(forwarder)
		refreshScreen()

		go func() {
			events := ui.EventSource{
				Events: forwarder.events(),
				EventHandledCallback: func(e termbox.Event) error {
					return refreshScreen()
				},
			}
			prompt.OnFocus(events)
			conf, cancel := prompt.Text()
			f(h)
			widgets.remove(prompt)
			if cancel || (conf != "y" && conf != "Y") {

				return
			}

			dry.actionMessage(id, "Removing")
			err := dry.dockerDaemon.Rm(id)
			if err == nil {
				dry.actionMessage(id, "removed")
				f(viewsToHandlers[Main])
				dry.SetViewMode(Main)
				refreshScreen()
			} else {
				dry.errorMessage(id, "removing", err)
			}

		}()

	case docker.STATS:
		forwarder := newEventForwarder()
		f(forwarder)
		refreshScreen()

		h.dry.SetViewMode(NoView)
		statsChan := dry.dockerDaemon.OpenChannel(container)
		go statsScreen(container, statsChan, screen, forwarder.events(),
			func() {
				h.dry.SetViewMode(ContainerMenu)
				f(h)
				refreshScreen()
			})

	case docker.INSPECT:
		forwarder := newEventForwarder()
		f(forwarder)
		refreshScreen()

		err := inspect(
			h.screen,
			forwarder.events(),
			func(id string) (interface{}, error) {
				return h.dry.dockerDaemon.Inspect(id)
			},
			func() {
				h.dry.SetViewMode(ContainerMenu)
				f(h)
				refreshScreen()
			})(id)

		if err != nil {
			dry.appmessage(
				fmt.Sprintf("Error inspecting container: %s", err.Error()))
			return
		}
	case docker.HISTORY:
		history, err := dry.dockerDaemon.History(container.ImageID)

		if err == nil {
			renderer := appui.NewDockerImageHistoryRenderer(history)
			forwarder := newEventForwarder()
			f(forwarder)
			go appui.Less(renderer, screen, forwarder.events(), func() {
				h.dry.SetViewMode(ContainerMenu)
				f(h)
				refreshScreen()
			})
		} else {
			dry.appmessage(
				fmt.Sprintf("Error showing image history: %s", err.Error()))
		}
	}
}
