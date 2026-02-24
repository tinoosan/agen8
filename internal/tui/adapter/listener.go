package adapter

import (
	"context"
	"encoding/json"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

type EventPushedMsg struct {
	Record types.EventRecord
	Ch     chan types.EventRecord
	ErrCh  chan error
}

type NotificationConnErrorMsg struct {
	Err error
}

func StartNotificationListenerCmd(endpoint string) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan types.EventRecord, 100)
		errCh := make(chan error, 1)

		go func() {
			err := protocol.RunNotificationListener(context.Background(), endpoint, func(msg protocol.Message) error {
				if msg.Method == protocol.MethodNotifyEventAppend {
					var ev types.EventRecord
					if err := json.Unmarshal(msg.Params, &ev); err == nil {
						ch <- ev
					}
				}
				return nil
			})
			errCh <- err
		}()

		select {
		case ev := <-ch:
			return EventPushedMsg{Record: ev, Ch: ch, ErrCh: errCh}
		case err := <-errCh:
			return NotificationConnErrorMsg{Err: err}
		}
	}
}

func WaitForNextNotificationCmd(ch chan types.EventRecord, errCh chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case ev := <-ch:
			return EventPushedMsg{Record: ev, Ch: ch, ErrCh: errCh}
		case err := <-errCh:
			return NotificationConnErrorMsg{Err: err}
		}
	}
}
