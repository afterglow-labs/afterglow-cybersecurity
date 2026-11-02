package server

import "github.com/Autumn-27/artex/locale"

func trMsg(message string) string { return locale.Text(locale.ServerDefault(), message) }
