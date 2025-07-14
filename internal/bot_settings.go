package internal

import (
	"fmt"
	"strconv"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/internal/consts"
	"zakirullin/stuffbot/internal/userconfig"
	"zakirullin/stuffbot/pkg/tg"
	"zakirullin/stuffbot/pkg/txt"
)

const (
	addBtn = "➕"
	delBtn = "➖"
)

func (b *Bot) showSettings(params []string) error {
	var kb tg.Keyboard
	kb.AddRow(tg.NewBtn(txt.Emoji(i18n.Emoji("chat"), b.tr("Saved messages mode")), tg.NewCmd(consts.CmdChatMode, nil)))
	kb.AddRow(tg.NewBtn(txt.Emoji(i18n.Emoji("brain"), b.tr("Full mode")), tg.NewCmd(consts.CmdFullMode, nil)))
	kb.AddRow(tg.NewBtn(txt.Emoji(i18n.Emoji("notes"), b.tr("Notes mode")), tg.NewCmd(consts.CmdNotesOnlyMode, nil)))
	kb.AddRow(tg.NewBtn(txt.Emoji(i18n.Emoji("tasks"), b.tr("Tasks mode")), tg.NewCmd(consts.CmdTasksOnlyMode, nil)))
	kb.AddRow(tg.NewBtn(txt.Emoji(i18n.Emoji("journal"), b.tr("Journal mode")), tg.NewCmd(consts.CmdJournalOnlyMode, nil)))
	kb.AddRow(tg.NewBtn("-", tg.NewCmd(consts.CmdDoNothing, nil)))
	kb.AddRow(tg.NewBtn(i18n.StrQuickBtns, tg.NewCmd(consts.CmdShowQuickBtnsSettings, nil)))
	kb.AddRow(tg.NewBtn(i18n.StrMoveToBtns, tg.NewCmd(consts.CmdShowMoveToBtnsSettings, nil)))
	kb.AddRow(tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil)))

	err := b.showHTML("Settings:", &kb)
	if err != nil {
		return fmt.Errorf("showSettings : %w", err)
	}

	return nil
}

func (b *Bot) showQuickBtnsSettings(params []string) error {
	var kb tg.Keyboard

	// Step 1. Append all buttons that are already chosen by user
	var usedCmds []string

	// We iterate through hardcoded panel to preserve order of buttons in UI
	cmds, err := b.cfg.QuickCmds()
	if err != nil {
		return fmt.Errorf("can't get quick cmds: %w", err)
	}

	for _, cmd := range cmds {
		for _, btn := range userconfig.AvailableQuickBtns {
			if btn.Cmd.Name != cmd {
				continue
			}

			name := fmt.Sprintf("%s %s %s", i18n.Emoji(btn.Name), btn.Name, delBtn)
			enabledCmd := tg.NewCmd(consts.CmdDelFromQuickBtns, []string{btn.Cmd.Name})
			kb.AddRow(tg.NewBtn(name, enabledCmd))
			usedCmds = append(usedCmds, cmd)
			break
		}
	}

	kb.AddRow(tg.NewBtn("-", tg.NewCmd(consts.CmdDoNothing, nil)))

	// Step 2. now, let's fill buttons that are not disabled...
	for _, btn := range userconfig.AvailableQuickBtns {
		// Check if command is enabled
		cmdUsed := false
		for _, usedCmd := range usedCmds {
			if btn.Cmd.Name == usedCmd {
				cmdUsed = true
			}
		}
		if cmdUsed {
			continue
		}
		// Command is not enabled, so add it to disabled list
		name := fmt.Sprintf("%s %s %s", i18n.Emoji(btn.Name), btn.Name, addBtn)
		disabledCmd := tg.NewCmd(consts.CmdAddToQuickBtns, []string{btn.Cmd.Name})
		kb.AddRow(tg.NewBtn(name, disabledCmd))
	}

	kb.AddRow(tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil)))

	text := fmt.Sprintf("Configure quick buttons (%s = add to quick buttons, %s = to remove from quick buttons):", addBtn, delBtn)
	err = b.showHTML(text, &kb)
	if err != nil {
		return fmt.Errorf("configureQuickPanel : %w", err)
	}

	return nil
}

func (b *Bot) addToQuickBtns(params []string) error {
	cmd := params[0]

	// Search whether a command is valid
	found := false
	for _, btn := range userconfig.AvailableQuickBtns {
		if btn.Cmd.Name == cmd {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("unknown command: %s", cmd)
	}

	err := b.cfg.AddQuickCmd(cmd)
	if err != nil {
		return fmt.Errorf("can't add to quick buttons: %w", err)
	}

	return b.showQuickBtnsSettings([]string{})
}

func (b *Bot) delFromQuickBtns(params []string) error {
	cmd := params[0]

	_ = b.cfg.DelQuickCmd(cmd)

	return b.showQuickBtnsSettings([]string{})
}

func (b *Bot) quickBtns() []tg.Btn {
	quickBtnsRow := tg.NewRow()
	// We can tolerate missing quick btns
	cmds, _ := b.cfg.QuickCmds()
	for _, cmd := range cmds {
		for _, btn := range userconfig.AvailableQuickBtns {
			if btn.Cmd.Name == cmd {
				if btn.Cmd.Name == consts.CmdWebAppHabits {
					habitsUrl := fmt.Sprintf("https://%s/habits_v2/%d", config.BotCfg.ApiHost, b.userID)
					btn.Cmd.Params = []string{habitsUrl}
				}
				btn.Name = i18n.Emoji(btn.Name)

				quickBtnsRow = append(quickBtnsRow, btn)
				break
			}
		}
	}

	return quickBtnsRow
}

// A little copy-paste from showQuickBtnsSettings
func (b *Bot) showMoveToBtnsSettings(params []string) error {
	var kb tg.Keyboard

	// Step 1. Append all buttons that are already chosen by user
	var usedCmds []string

	// We iterate through hardcoded panel to preserve order of buttons in UI
	cmds, err := b.cfg.MoveToCmds()
	if err != nil {
		return fmt.Errorf("can't get move to cmds: %w", err)
	}
	for _, cmd := range cmds {
		for _, btn := range userconfig.AvailableMoveToBtns {
			if btn.Cmd.Name != cmd {
				continue
			}

			name := txt.Emoji(delBtn, btn.Name)
			enabledCmd := tg.NewCmd(consts.CmdDelFromMoveToBtns, []string{btn.Cmd.Name})
			kb.AddRow(tg.NewBtn(name, enabledCmd))
			usedCmds = append(usedCmds, cmd)
			break
		}
	}

	kb.AddRow(tg.NewBtn("-", tg.NewCmd(consts.CmdDoNothing, nil)))

	// Step 2. now, let's fill buttons that are not disabled...
	for _, btn := range userconfig.AvailableMoveToBtns {
		// Check if command is enabled
		cmdUsed := false
		for _, usedCmd := range usedCmds {
			if btn.Cmd.Name == usedCmd {
				cmdUsed = true
			}
		}
		if cmdUsed {
			continue
		}
		// Command is not enabled, so add it to disabled list
		name := txt.Emoji(addBtn, btn.Name)
		disabledCmd := tg.NewCmd(consts.CmdAddToMoveToBtns, []string{btn.Cmd.Name})
		kb.AddRow(tg.NewBtn(name, disabledCmd))
	}

	kb.AddRow(tg.NewBtn(i18n.StrToday, tg.NewCmd(consts.CmdShowToday, nil)))

	text := fmt.Sprintf("Configure quick panel (%s = add to panel, %s = to remove):", addBtn, delBtn)
	err = b.showHTML(text, &kb)
	if err != nil {
		return fmt.Errorf("configureQuickPanel : %w", err)
	}

	return nil
}

func (b *Bot) addToMoveToBtns(params []string) error {
	cmd := params[0]

	err := b.cfg.AddMoveToCmd(cmd)
	if err != nil {
		return fmt.Errorf("can't add to move to buttons: %w", err)
	}

	return b.showMoveToBtnsSettings([]string{})
}

func (b *Bot) delFromMoveToBtns(params []string) error {
	cmd := params[0]

	err := b.cfg.DelMoveToCmd(cmd)
	if err != nil {
		return fmt.Errorf("button doesn't exist in user's prefs: %s", params[0])
	}

	return b.showMoveToBtnsSettings([]string{})
}

func (b *Bot) moveToBtns(msgIndex int) []tg.Btn {
	moveToBtns := tg.NewRow()

	cmds, err := b.cfg.MoveToCmds()
	if err != nil {
		return nil
	}

	for _, cmd := range cmds {
		for _, btn := range userconfig.AvailableMoveToBtns {
			if btn.Cmd.Name == cmd {
				btn.Cmd.Params = []string{strconv.Itoa(msgIndex)}
				moveToBtns = append(moveToBtns, btn)
				break
			}
		}
	}

	return moveToBtns
}
