package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

type Messenger struct {
	client *lark.Client
}

func NewMessenger(appID, appSecret string) *Messenger {
	return &Messenger{
		client: lark.NewClient(appID, appSecret),
	}
}

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
