package businessflow

import "sync"

var (
	shortLinkGenMutex sync.Mutex
)

func lockShortLinkGen() {
	shortLinkGenMutex.Lock()
}

func unlockShortLinkGen() {
	shortLinkGenMutex.Unlock()
}
