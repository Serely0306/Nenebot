package core

import "sync"

var (
	configMutex sync.RWMutex
	configPath  string
	CONFIG      Config

	botInfoMu   sync.RWMutex
	botNickname string

	configResolver   func(*Config) error
	configReloadHook func()
)

func SetConfigResolver(fn func(*Config) error) {
	configResolver = fn
}

func SetConfigReloadHook(fn func()) {
	configReloadHook = fn
}

func SetBotNickname(nickname string) {
	botInfoMu.Lock()
	defer botInfoMu.Unlock()
	botNickname = nickname
}

func GetBotNickname() string {
	botInfoMu.RLock()
	defer botInfoMu.RUnlock()
	return botNickname
}

func GetConfigPath() string {
	return configPath
}

func IsSuperUser(userID int64) bool {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return CONFIG.Server.CommandAuth.IsSuperUser(userID)
}
