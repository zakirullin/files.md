package plugins

import "zakirullin/stuffbot/pkg/tg"

type TGInterface interface {
	Send(userID int64, text string, kb *tg.Keyboard, markup string) (int, error)
}
