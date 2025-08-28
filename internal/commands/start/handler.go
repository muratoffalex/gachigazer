package start

import (
	"fmt"

	"github.com/muratoffalex/gachigazer/internal/app/di"
	"github.com/muratoffalex/gachigazer/internal/commands/base"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

const CommandName = "start"

type Command struct {
	*base.Command
}

func New(di *di.Container) *Command {
	cmd := &Command{}
	cmd.Command = base.NewCommand(cmd, di)
	return cmd
}

func (c *Command) Name() string {
	return CommandName
}

func (c *Command) Execute(update telegram.Update) error {
	msg := telegram.NewMessage(
		update.Message.Chat.ID,
		fmt.Sprintf(
			"User ID: `%d`\nChat ID: `%d`",
			update.Message.From.ID,
			update.Message.Chat.ID,
		),
		update.Message.MessageID,
	)
	msg.ParseMode = telegram.ModeMarkdownV2

	_, err := c.Tg.Send(msg)
	if err != nil {
		c.Logger.WithError(err).Error("Failed to send message")
		return err
	}

	return nil
}
