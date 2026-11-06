package server

import "github.com/afterglow-labs/afterglow-cybersecurity/locale"

func trMsg(message string) string { return locale.Text(locale.ServerDefault(), message) }
