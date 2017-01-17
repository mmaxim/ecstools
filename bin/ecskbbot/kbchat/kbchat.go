package kbchat

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

type API struct {
	input  io.Writer
	output *bufio.Scanner
}

func NewAPI(input io.Writer, output *bufio.Scanner) *API {
	return &API{
		input:  input,
		output: output,
	}
}

func (a *API) GetConversations(unreadOnly bool) ([]Conversation, error) {
	list := fmt.Sprintf("{\"method\":\"list\", \"params\": { \"options\": { \"unread_only\": %v}}}", unreadOnly)
	if _, err := io.WriteString(a.input, list); err != nil {
		return nil, err
	}
	a.output.Scan()

	var inbox Inbox
	inboxRaw := a.output.Text()
	if err := json.Unmarshal([]byte(inboxRaw[:]), &inbox); err != nil {
		return nil, err
	}
	return inbox.Result.Convs, nil
}

func (a *API) GetTextMessages(convID string, unreadOnly bool) ([]Message, error) {
	read := fmt.Sprintf("{\"method\": \"read\", \"params\": {\"options\": {\"conversation_id\": \"%s\", \"unread_only\": %v}}}", convID, unreadOnly)
	if _, err := io.WriteString(a.input, read); err != nil {
		return nil, err
	}
	a.output.Scan()

	var thread Thread
	if err := json.Unmarshal([]byte(a.output.Text()), &thread); err != nil {
		return nil, fmt.Errorf("unable to decode thread: %s", err.Error())
	}

	var res []Message
	for _, msg := range thread.Result.Messages {
		if msg.Msg.Content.Type == "text" {
			res = append(res, msg.Msg)
		}
	}

	return res, nil
}

func (a *API) SendMessage(convID string, body string) error {
	send := fmt.Sprintf("{\"method\": \"send\", \"params\": {\"options\": {\"conversation_id\": \"%s\", \"message\": {\"body\": \"%s\"}}}}", convID, body)
	if _, err := io.WriteString(a.input, send); err != nil {
		return err
	}
	a.output.Scan()
	return nil
}
