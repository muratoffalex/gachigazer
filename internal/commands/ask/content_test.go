package ask

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageContentSenderTags(t *testing.T) {
	content := &MessageContent{
		Text:     "hi",
		UserInfo: userInfo{Name: "Alexander", EncodedID: "abc", SenderTag: "Чилловый парень"},
		ReplyMsgContent: &MessageContent{
			Text:     "hello",
			UserInfo: userInfo{Name: "Other", EncodedID: "def", SenderTag: "tag\n\"quoted\""},
		},
	}
	text := content.GetMessageContent()
	require.Contains(t, text, `[USER: Alexander(abc) tag="Чилловый парень" @`)
	require.Contains(t, text, `[REPLY TO: Other(def) tag="tag\n\"quoted\"" @`)
	withoutTag := &MessageContent{Text: "hi", UserInfo: userInfo{Name: "Alexander", EncodedID: "abc"}}
	require.Contains(t, withoutTag.GetMessageContent(), "[USER: Alexander(abc) @")
	require.NotContains(t, withoutTag.GetMessageContent(), "tag=")
}
