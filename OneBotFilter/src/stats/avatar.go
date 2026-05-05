package stats

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const avatarCacheTTL = 24 * time.Hour

type cachedAvatar struct {
	data      []byte
	updatedAt time.Time
}

var (
	avatarHTTPClient = &http.Client{Timeout: 2500 * time.Millisecond}
	avatarCacheMu    sync.RWMutex
	avatarCache      = make(map[string]cachedAvatar)
)

func fetchUserAvatar(userID int64) []byte {
	if userID <= 0 {
		return nil
	}
	return fetchAvatar("user", userID, userAvatarURL(userID))
}

func fetchGroupAvatar(groupID int64) []byte {
	if groupID <= 0 {
		return nil
	}
	return fetchAvatar("group", groupID, groupAvatarURL(groupID))
}

func userAvatarURL(userID int64) string {
	return fmt.Sprintf("https://q4.qlogo.cn/g?b=qq&nk=%d&s=100", userID)
}

func groupAvatarURL(groupID int64) string {
	return fmt.Sprintf("https://p.qlogo.cn/gh/%d/%d/100", groupID, groupID)
}

func fetchAvatar(kind string, id int64, url string) []byte {
	key := fmt.Sprintf("%s:%d", kind, id)
	if data, ok := lookupAvatarCache(key); ok {
		return data
	}

	resp, err := avatarHTTPClient.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || len(data) == 0 {
		return nil
	}
	storeAvatarCache(key, data)
	return data
}

func lookupAvatarCache(key string) ([]byte, bool) {
	avatarCacheMu.RLock()
	entry, ok := avatarCache[key]
	avatarCacheMu.RUnlock()
	if !ok || time.Since(entry.updatedAt) > avatarCacheTTL || len(entry.data) == 0 {
		return nil, false
	}
	return append([]byte(nil), entry.data...), true
}

func storeAvatarCache(key string, data []byte) {
	if len(data) == 0 {
		return
	}
	avatarCacheMu.Lock()
	avatarCache[key] = cachedAvatar{
		data:      append([]byte(nil), data...),
		updatedAt: time.Now(),
	}
	avatarCacheMu.Unlock()
}

func fetchUserAvatars(userIDs []int64) map[int64][]byte {
	result := make(map[int64][]byte, len(userIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		userID := userID
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			data := fetchUserAvatar(userID)
			<-sem
			if len(data) == 0 {
				return
			}
			mu.Lock()
			result[userID] = data
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result
}

func fetchGroupAvatars(groupIDs []int64) map[int64][]byte {
	result := make(map[int64][]byte, len(groupIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		groupID := groupID
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			data := fetchGroupAvatar(groupID)
			<-sem
			if len(data) == 0 {
				return
			}
			mu.Lock()
			result[groupID] = data
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result
}
