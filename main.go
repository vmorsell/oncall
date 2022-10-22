package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func scheduleTable(schedule Schedule) *tview.Table {
	table := tview.NewTable()
	table.SetBorders(false)

	headers := []string{
		"Name",
		"Start Time",
		"End Time",
	}
	for i, h := range headers {
		cell := tview.NewTableCell(h)
		cell.SetAttributes(tcell.AttrBold)
		cell.SetSelectable(false)
		if i == 0 {
			cell.SetExpansion(1)
		}

		table.SetCell(0, i, cell)
	}

	for i, p := range schedule.periods {
		values := []string{
			emailToName(p.email),
			fmtTime(p.starts),
			fmtTime(p.ends),
		}
		for j, v := range values {
			table.SetCell(i+1, j, tview.NewTableCell(v))
		}
	}

	return table
}

func fmtTime(t time.Time) string {
	if t.Before(time.Now()) {
		return "Now"
	}
	return t.Local().Format("Mon Jan _2 15:04")
}

func fmtDelay(v int) string {
	return fmt.Sprintf("%d min", v/60)
}

func emailToName(v string) string {
	v = strings.Split(v, "@")[0]
	v = strings.ReplaceAll(v, ".", " ")
	v = strings.Title(v)
	return v
}

func main() {
	ctx := context.Background()

	cfg, err := readConfig()
	if err != nil {
		log.Fatalf("read config: %v", err)
	}
	og := NewOpsGenieClient(cfg.OpsGenie.APIKey)

	app := tview.NewApplication()

	layout := tview.NewFlex().SetDirection(tview.FlexRow)
	for _, name := range cfg.TeamNames {
		schedule, err := og.getSchedule(ctx, name, 3)
		if err != nil {
			log.Fatalf("get schedule: %v", err)
		}
		table := scheduleTable(schedule)
		table.SetBorder(true)
		table.SetTitle(fmt.Sprintf(" %s: %s ", fmtDelay(schedule.delay), schedule.name))
		layout.AddItem(table, 0, 1, true)
	}

	app.SetRoot(layout, true)
	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
