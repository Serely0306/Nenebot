package aichat

import (
	zero "github.com/wdvxdr1123/ZeroBot"
)

type storage struct {
	groupID int64
	fs      *FileStorage
}

func newstorage(ctx *zero.Ctx, gid int64) (storage, error) {
	fs := GetFileStorage()
	return storage{
		groupID: gid,
		fs:      fs,
	}, nil
}

func (s storage) rate() uint8 {
	config := s.fs.Get(s.groupID)
	return config.Rate
}

func (s storage) temp() float32 {
	config := s.fs.Get(s.groupID)
	temp := int(config.Temp)
	if temp <= 0 {
		temp = 70
	}
	if temp > 100 {
		temp = 100
	}
	return float32(temp) / 100
}

func (s storage) noagent() bool {
	config := s.fs.Get(s.groupID)
	return config.NoAgent
}

func (s storage) norecord() bool {
	config := s.fs.Get(s.groupID)
	return config.NoRecord
}

func (s storage) noreplyat() bool {
	config := s.fs.Get(s.groupID)
	return config.NoReplyAt
}
