package userconfig

import (
	"fmt"

	"zakirullin/stuffbot/i18n"
	"zakirullin/stuffbot/server/consts"
	"zakirullin/stuffbot/pkg/tg"
)

var AvailableMoveToBtns = []tg.Btn{
	tg.NewBtn(i18n.StrToTomorrow, tg.NewCmd(consts.CmdScheduleForTmrw, nil)),
	tg.NewBtn(i18n.StrToLater, tg.NewCmd(consts.CmdMoveToLater, nil)),
	tg.NewBtn(i18n.StrToADay, tg.NewCmd(consts.CmdShowScheduleForDay, nil)),
	tg.NewBtn(i18n.StrToFile, tg.NewCmd(consts.CmdShowMoveToDirOrFile, nil)),
	tg.NewBtn(i18n.StrToJournal, tg.NewCmd(consts.CmdMoveToJournal, nil)),
	tg.NewBtn(i18n.StrToRead, tg.NewCmd(consts.CmdMoveToRead, nil)),
	tg.NewBtn(i18n.StrToWatch, tg.NewCmd(consts.CmdMoveToWatch, nil)),
	tg.NewBtn(i18n.StrToShop, tg.NewCmd(consts.CmdMoveToShop, nil)),
	tg.NewBtn(i18n.StrToChecklist, tg.NewCmd(consts.CmdShowMoveToChecklist, nil)),
}

func (c *Config) AddMoveToCmd(cmd string) error {
	lock := c.userLock()
	lock.Lock()
	defer lock.Unlock()

	cfg, err := c.read(c.filename)
	if err != nil {
		return fmt.Errorf("can't add move to cmd: can't read config: %w", err)
	}

	for _, existingCmd := range cfg.MoveToCmds {
		if existingCmd == cmd {
			return nil
		}
	}

	cfg.MoveToCmds = append(cfg.MoveToCmds, cmd)
	err = c.write(cfg)
	if err != nil {
		return fmt.Errorf("can't add move to cmd: can't write config: %w", err)
	}

	return nil
}

func (c *Config) MoveToCmds() ([]string, error) {
	cfg, err := c.read(c.filename)
	if err != nil {
		return nil, fmt.Errorf("can't get move to cmds: can't read config: %w", err)
	}

	return cfg.MoveToCmds, nil
}

func (c *Config) DelMoveToCmd(cmd string) error {
	lock := c.userLock()
	lock.Lock()
	defer lock.Unlock()

	cfg, err := c.read(c.filename)
	if err != nil {
		return fmt.Errorf("can't del move to cmd: can't read config: %w", err)
	}

	newCmds := []string{}
	for _, curMoveToCmd := range cfg.MoveToCmds {
		if curMoveToCmd != cmd {
			newCmds = append(newCmds, curMoveToCmd)
		}
	}
	cfg.MoveToCmds = newCmds

	err = c.write(cfg)
	if err != nil {
		return fmt.Errorf("can't del move to cmd: can't write config: %w", err)
	}

	return nil
}
