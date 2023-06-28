package tg

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/require"
)

func TestSend(t *testing.T) {
	r := require.New(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"ok":true, "result": {"id": -2}}`))

		if req.URL.Path == "/sendMessage" {
			body, err := io.ReadAll(req.Body)
			r.NoError(err)
			r.Equal("chat_id=-1&entities=null&parse_mode=Html&text=t", string(body))
		}
	}))

	api, _ := tgbotapi.NewBotAPIWithAPIEndpoint("", ts.URL+"%s/%s")

	tg := NewTG(api)

	tg.SendHTML(-1, "t", nil)
}

func TestEdit(t *testing.T) {
	r := require.New(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"ok":true, "result": {"id": -2}}`))

		if req.URL.Path == "/editMessage" {
			body, err := io.ReadAll(req.Body)
			r.NoError(err)
			r.Equal("chat_id=-1&entities=null&parse_mode=Html&text=t", string(body))
		}
	}))

	api, _ := tgbotapi.NewBotAPIWithAPIEndpoint("", ts.URL+"%s/%s")

	tg := NewTG(api)

	tg.Edit(-1, -2, "t", nil, MarkupHTML)
}

func TestDel(t *testing.T) {
	r := require.New(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"ok":true, "result": {"id": -2}}`))

		if req.URL.Path == "/deleteMessage" {
			body, err := io.ReadAll(req.Body)
			r.NoError(err)
			r.Equal("chat_id=-1&message_id=-2", string(body))
		}
	}))

	api, _ := tgbotapi.NewBotAPIWithAPIEndpoint("", ts.URL+"%s/%s")

	tg := NewTG(api)

	tg.Del(-1, -2)
}
