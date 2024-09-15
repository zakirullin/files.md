package gui

import (
	"io"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"zakirullin/stuffbot/internal"
	"zakirullin/stuffbot/pkg/tg"
	"zakirullin/stuffbot/pkg/txt"
)

type ChatGUI struct {
	userID    int64
	messages  *fyne.Container
	scroll    *container.Scroll
	window    fyne.Window
	entry     *entry
	updater   func(updInterface internal.UpdInterface) error
	container *fyne.Container
}

var Chat *ChatGUI

const (
	width           = 560
	height          = 580
	maxCharsPerLine = 50
)

func NewGui(userID int64, updater func(u internal.UpdInterface) error) *ChatGUI {
	return &ChatGUI{userID: userID, messages: container.NewVBox(), entry: newEntry(), updater: updater}
}

func (c *ChatGUI) Run(startupCMD tg.Cmd) {
	a := app.New()
	c.window = a.NewWindow("Files.md")

	sendBtn := newButton("✉️", func() {
		if c.entry.MultiLine {
			c.entry.Resize(fyne.NewSize(c.entry.Size().Width, c.entry.Size().Height/2))
			c.entry.Refresh()
			c.entry.MultiLine = false
		}
		sendMsg()
	})

	// Make sure the entry field takes all available width
	inputLine := container.New(layout.NewBorderLayout(nil, nil, nil, sendBtn), c.entry, sendBtn)
	c.scroll = container.NewVScroll(container.NewVBox(layout.NewSpacer(), c.messages))
	c.container = container.New(layout.NewBorderLayout(nil, inputLine, nil, nil), c.scroll, inputLine)

	c.window.SetContent(c.container)
	c.window.Resize(fyne.NewSize(width, height))
	c.window.Show()
	c.window.Canvas().Focus(c.entry)

	c.updater(tg.NewFakeUpdCmd(1, startupCMD))
	a.Run()
}

func (c *ChatGUI) Send(_ int64, text string, kb *tg.Keyboard, markup string) (int, error) {
	text = txt.StripHTMLTags(text)
	if len(text) == 0 {
		return 0, nil
	}

	// We don't need a separate container here I beleive
	btnsContainer := container.NewVBox()
	var msgContainer *fyne.Container
	if len(text) > maxCharsPerLine {
		text = txt.SplitLongLines(text, maxCharsPerLine)
		multilineEntry := widget.NewMultiLineEntry()
		multilineEntry.Text = text
		multilineEntry.SetMinRowsVisible(strings.Count(text, "\n") + 1)
		msgContainer = container.New(layout.NewBorderLayout(multilineEntry, btnsContainer, nil, nil))
		msgContainer.Add(multilineEntry)
		msgContainer.Add(btnsContainer)
	} else {
		label := widget.NewLabel(text)
		msgContainer = container.New(layout.NewBorderLayout(label, btnsContainer, nil, nil))
		msgContainer.Add(label)
		msgContainer.Add(btnsContainer)
	}
	c.attachKeyboard(kb, btnsContainer)

	c.messages.Add(msgContainer)
	c.scroll.Refresh()
	c.scroll.ScrollToBottom()

	return 0, nil
}

func (c *ChatGUI) Edit(userID int64, _ int, text string, kb *tg.Keyboard, markup string) error {
	if len(text) == 0 {
		return nil
	}

	c.messages.RemoveAll()
	c.Send(userID, text, kb, markup)

	return nil
}

func (c *ChatGUI) Del(_ int64, _ int) error {
	return nil
}

func (c *ChatGUI) AnswerCallbackQuery(_ string, msg string) error {
	if len(msg) == 0 {
		return nil
	}
	msg = "🎉"

	toast := canvas.NewText(msg, theme.Color(theme.ColorNamePrimary))
	toast.Alignment = fyne.TextAlignCenter
	Chat.container.Add(toast)
	toast.Refresh()

	Chat.window.Canvas().Content()
	go func() {
		time.Sleep(2 * time.Second)
		toast.Hide()
	}()

	return nil
}

func (c *ChatGUI) AnswerInlineQuery(_ string, _ []interface{}, _ int, _ string) error {
	return nil
}

func (c *ChatGUI) DownloadFile(_ string, _ io.Writer) (string, error) {
	return "", nil
}

func (c *ChatGUI) attachKeyboard(kb *tg.Keyboard, msgContainer *fyne.Container) {
	if kb == nil {
		return
	}

	btnCallback := func(cmd tg.Cmd) func() {
		return func() {
			c.updater(tg.NewFakeUpdCmd(1, cmd))
			c.scroll.Refresh()
			c.scroll.ScrollToBottom()
		}
	}
	for _, row := range kb.Btns {
		switch row.(type) {
		case tg.Btn:
			b := row.(tg.Btn)
			btn := newButton(b.Name, btnCallback(b.Cmd))
			msgContainer.Add(btn)
		case []tg.Btn:
			btns := row.([]tg.Btn)
			rowContainer := container.New(layout.NewGridLayoutWithColumns(len(btns)))
			for _, b := range btns {
				rowContainer.Add(newButton(b.Name, btnCallback(b.Cmd)))
			}
			msgContainer.Add(rowContainer)
		}
	}
}

func sendMsg() {
	msg := strings.TrimSpace(Chat.entry.Text)
	if len(msg) > 0 {
		if (msg[0] == '/') && (len(msg) > 1) {
			Chat.updater(tg.NewFakeUpdCmd(1, tg.NewCmd(msg[1:], nil)))
		} else {
			Chat.messages.RemoveAll()
			Chat.updater(tg.NewFakeUpd(1, msg))
		}
	}
	Chat.entry.SetText("")
}
