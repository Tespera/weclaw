package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"weclaw/ilink"
	"weclaw/messaging"
)

var (
	sendTo      string
	sendText    string
	sendMedia   []string
	sendNoQueue bool
)

func init() {
	sendCmd.Flags().StringVar(&sendTo, "to", "", "Target user ID (ilink user ID)")
	sendCmd.Flags().StringVar(&sendText, "text", "", "Message text to send")
	sendCmd.Flags().StringArrayVar(&sendMedia, "media", nil, "Image/video/file to send: URL or local path (repeatable)")
	sendCmd.Flags().BoolVar(&sendNoQueue, "no-queue", false, "Do not keep an undelivered message for automatic resend")
	sendCmd.MarkFlagRequired("to")
	rootCmd.AddCommand(sendCmd)
}

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to a WeChat user",
	Long: `Send a message to a WeChat user.

WeChat only lets the bot message a user within about 24 hours of the user's
last message to it, and only about 10 messages in a row without a reply.
When a send is refused for that reason, or the WeChat server cannot be
reached, the message is kept and resent automatically: on the user's next
message, or once the network is back. Exit codes: 0 sent, 3 refused by
WeChat (queued unless --no-queue), 4 network failure (queued unless
--no-queue), 1 other errors.`,
	SilenceUsage:  true,
	SilenceErrors: true, // Execute prints the error once
	Example: `  weclaw send --to "user_id@im.wechat" --text "Hello"
  weclaw send --to "user_id@im.wechat" --media "https://example.com/image.png"
  weclaw send --to "user_id@im.wechat" --text "See this" --media "https://example.com/image.png"
  weclaw send --to "user_id@im.wechat" --media ~/Desktop/report.pdf --media ./chart.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if sendText == "" && len(sendMedia) == 0 {
			return fmt.Errorf("at least one of --text or --media is required")
		}
		// Resolve every media item before sending anything, so a bad path
		// does not leave the recipient with a partial message.
		media := make([]messaging.MediaRef, 0, len(sendMedia))
		for _, ref := range sendMedia {
			m, err := messaging.ResolveMediaRef(ref)
			if err != nil {
				return err
			}
			media = append(media, m)
		}

		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		accounts, err := ilink.LoadAllCredentials()
		if err != nil {
			return fmt.Errorf("load credentials: %w", err)
		}
		if len(accounts) == 0 {
			return fmt.Errorf("no accounts found, run 'weclaw start' first")
		}

		client := ilink.NewClient(accounts[0])
		store, err := messaging.DefaultStore()
		if err != nil {
			log.Printf("state unavailable, sending without context token: %v", err)
			store = nil
		}

		d := messaging.NewDeliverer(client, store)
		err = d.Deliver(ctx, sendTo, messaging.Outgoing{Text: sendText, Media: media}, !sendNoQueue, func(part string) {
			if part == "text" {
				fmt.Println("Text sent")
			} else {
				fmt.Printf("Media sent: %s\n", part)
			}
		})
		if de, ok := messaging.IsDeliveryError(err); ok {
			fmt.Fprintln(os.Stderr, de.Error())
			os.Exit(de.ExitCode())
		}
		if err != nil {
			return fmt.Errorf("send failed: %w", err)
		}

		return nil
	},
}
