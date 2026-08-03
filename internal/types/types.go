// Copyright 2026 qmahyar and contributors. Licensed under Apache-2.0. See LICENSE.

package types

type MirrorStatus struct {
	Accounts    int    `json:"accounts"`
	Chats       int    `json:"chats"`
	Messages    int    `json:"messages"`
	DbSizeBytes int    `json:"db_size_bytes"`
	LastSyncAt  string `json:"last_sync_at"`
}
