package appui

import (
	"bytes"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/docker/go-units"
	"github.com/gosuri/uitable/util/strutil"
	drydocker "github.com/moncho/dry/docker"
	"github.com/moncho/dry/ui"
)

type statsRenderer struct {
	stats *drydocker.Stats
}

//NewDockerStatsRenderer creates renderer for docker stats
func NewDockerStatsRenderer(stats *drydocker.Stats) ui.Renderer {
	return &statsRenderer{
		stats: stats,
	}
}

//Render container stats
func (r *statsRenderer) Render() string {
	s := r.stats
	processList := r.stats.ProcessList

	buf := bytes.NewBufferString("")
	io.WriteString(buf, "\n")
	io.WriteString(buf, "\n")
	io.WriteString(buf, "<blue><b>STATS</></>\n")

	w := tabwriter.NewWriter(buf, 22, 0, 1, ' ', 0)
	io.WriteString(w, "<green>CONTAINER\tCOMMAND\t%%CPU\tMEM USAGE / LIMIT\t%%MEM\tNET I/O\tBLOCK I/O</>\n")
	io.WriteString(
		w,
		fmt.Sprintf("<white>%s\t%s\t%.2f\t%s / %s\t%.2f\t%s / %s\t%s / %s</>\n",
			s.CID,
			strutil.Resize(s.Command, 20),
			s.CPUPercentage,
			units.HumanSize(s.Memory), units.HumanSize(s.MemoryLimit),
			s.MemoryPercentage,
			units.HumanSize(s.NetworkRx), units.HumanSize(s.NetworkTx),
			units.HumanSize(s.BlockRead), units.HumanSize(s.BlockWrite)))
	if processList != nil {
		topRenderer := NewDockerTopRenderer(processList)
		io.WriteString(buf, "\n")
		io.WriteString(buf, "\n")
		io.WriteString(w, topRenderer.Render())
	}
	w.Flush()
	return buf.String()
}