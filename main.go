package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func scheduleTable(schedule Schedule) *tview.Table {
	table := tview.NewTable()
	table.SetBorders(false)

	headers := []string{
		"Start Time",
		"End Time",
		"Name",
	}
	for i, h := range headers {
		cell := tview.NewTableCell(h)
		cell.SetAttributes(tcell.AttrBold)
		cell.SetSelectable(false)
		cell.SetExpansion(1)

		table.SetCell(0, i, cell)
	}

	for i, p := range schedule.periods {
		values := []string{
			fmtTime(p.starts),
			fmtTime(p.ends),
			emailToName(p.email),
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

	menu := tview.NewTextView()
	pages := tview.NewPages()

	menu.SetDynamicColors(true)
	menu.SetRegions(true)
	menu.SetHighlightedFunc(func(added, _, _ []string) {
		pages.SwitchToPage(added[0])
	})

	for i, name := range cfg.TeamNames {
		schedule, err := og.getSchedule(ctx, name, 3)
		if err != nil {
			log.Fatalf("get schedule: %v", err)
		}
		pages.AddPage(strconv.Itoa(i), scheduleTable(schedule), true, i == 0)

		fmt.Fprintf(menu, `%d ["%d"][darkcyan]%s[white][""]  `, i+1, i, name)
	}
	menu.Highlight("0")

	layout := tview.NewFlex().SetDirection(tview.FlexRow)
	layout.AddItem(pages, 0, 1, true)
	layout.AddItem(menu, 1, 1, false)

	app := tview.NewApplication()

	// Listen to keyboard events.
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Page navigation with number keys.
		one := rune(49)
		max := one + rune(pages.GetPageCount()-1)

		if event.Rune() >= one && event.Rune() <= max {
			menu.Highlight(fmt.Sprint(event.Rune() - one)).ScrollToHighlight()
		}
		return event
	})

	app.SetRoot(layout, true)
	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
