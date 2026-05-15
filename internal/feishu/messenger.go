package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// Messenger wraps the Lark SDK client and sends plain-text messages to
// Feishu group chats.
type Messenger struct {
	client *lark.Client
}

// NewMessenger creates a Messenger backed by a Lark SDK client initialised
// with the given appID and appSecret credentials.
func NewMessenger(appID, appSecret string) *Messenger {
	return &Messenger{
		client: lark.NewClient(appID, appSecret),
	}
}

// SendText posts a plain-text message to the Feishu chat identified by
// chatID.  It returns an error if the Lark API call fails or the response
// indicates a non-success status.
func (m *Messenger) SendText(ctx context.Context, chatID, text string) error {
	contentBytes, _ := json.Marshal(map[string]string{
		"text": text,
	})

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType("text").
			Content(string(contentBytes)).
			Build()).
		Build()

	resp, err := m.client.Im.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("发送飞书消息失败: %w", err)
	}
	if !resp.Success() {
		return fmt.Errorf("发送飞书消息失败: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	return nil

}
