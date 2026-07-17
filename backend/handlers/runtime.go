package handlers

import (
	"geoguessme/internal/config"
	"geoguessme/internal/email"
	"geoguessme/internal/storage"
)

var (
	RuntimeConfig *config.Config
	MediaStore    storage.ObjectStore
	Mailer        email.Sender = email.Noop{}
)

func Configure(cfg *config.Config, store storage.ObjectStore, sender email.Sender) {
	RuntimeConfig = cfg
	MediaStore = store
	if sender != nil {
		Mailer = sender
	}
}
