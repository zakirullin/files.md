package userconfig

import (
	"fmt"

	"zakirullin/stuffbot/server/consts"
	"zakirullin/stuffbot/pkg/tg"
)

var AvailableQuickBtns = []tg.Btn{
	tg.NewBtn("Later", tg.NewCmd(consts.CmdLater, nil)),
	tg.NewBtn("Search", tg.NewCustomCmd(consts.CmdInlineQuerySearchEveryWhere, nil, tg.CmdTypeInlineQueryCurrentChat)),
	tg.NewBtn("Files", tg.NewCmd(consts.CmdShowFiles, nil)),
	tg.NewBtn("Checklists", tg.NewCmd(consts.CmdShowChecklists, nil)),
	tg.NewBtn("Postpone", tg.NewCmd(consts.CmdShowPostpone, nil)),
	tg.NewBtn("Read", tg.NewCmd(consts.CmdShowReadChecklist, nil)),
	tg.NewBtn("Watch", tg.NewCmd(consts.CmdShowWatchChecklist, nil)),
	tg.NewBtn("Shop", tg.NewCmd(consts.CmdShowShopChecklist, nil)),
	tg.NewBtn("Schedule", tg.NewCmd(consts.CmdShowSchedule, nil)),
	tg.NewBtn("Habits", tg.NewCustomCmd(consts.CmdWebAppHabits, nil, tg.CmdTypeWebApp)),
}

func (c *Config) AddQuickCmd(cmd string) error {
	lock := c.userLock()
	lock.Lock()
	defer lock.Unlock()

	cfg, err := c.read(c.filename)
	if err != nil {
		return fmt.Errorf("can't add quick cmd: can't read config: %w", err)
	}

	for _, existingCmd := range cfg.QuickCmds {
		if existingCmd == cmd {
			return nil
		}
	}

	cfg.QuickCmds = append(cfg.QuickCmds, cmd)
	err = c.write(cfg)
	if err != nil {
		return fmt.Errorf("can't add quick cmd: can't write config: %w", err)
	}

	return nil
}

func (c *Config) QuickCmds() ([]string, error) {
	cfg, err := c.read(c.filename)
	if err != nil {
		return nil, fmt.Errorf("can't get quick cmds: can't read config: %w", err)
	}

	return cfg.QuickCmds, nil
}

func (c *Config) DelQuickCmd(cmd string) error {
	lock := c.userLock()
	lock.Lock()
	defer lock.Unlock()

	cfg, err := c.read(c.filename)
	if err != nil {
		return fmt.Errorf("can't del quick cmd: can't read config: %w", err)
	}

	newCmds := []string{}
	for _, curQuickCmd := range cfg.QuickCmds {
		if curQuickCmd != cmd {
			newCmds = append(newCmds, curQuickCmd)
		}
	}
	cfg.QuickCmds = newCmds

	err = c.write(cfg)
	if err != nil {
		return fmt.Errorf("can't del quick cmd: can't write config: %w", err)
	}

	return nil
}
