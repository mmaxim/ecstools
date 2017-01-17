package kbchat

type Channel struct {
	Name      string `json:"name"`
	Public    bool   `json:"public"`
	TopicType string `json:"topic_type"`
}

type Conversation struct {
	Id      string  `json:"id"`
	Unread  bool    `json:"unread"`
	Channel Channel `json:"channel"`
}

type Result struct {
	Convs []Conversation `json:"conversations"`
}

type Inbox struct {
	Result Result `json:"result"`
}

type Text struct {
	Body string `json:"body"`
}

type Content struct {
	Type string `json:"type"`
	Text Text   `json:"text"`
}

type Message struct {
	Content Content `json:"content"`
}

type MessageHolder struct {
	Msg Message `json:"msg"`
}

type ThreadResult struct {
	Messages []MessageHolder `json:"messages"`
}

type Thread struct {
	Result ThreadResult `json:"result"`
}
