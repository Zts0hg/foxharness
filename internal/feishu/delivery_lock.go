package feishu

import (
	"context"
	"sync"
)

type deliveryPathLock struct {
	token chan struct{}
	refs  int
}

var deliveryPathLocks = struct {
	sync.Mutex
	byPath map[string]*deliveryPathLock
}{byPath: make(map[string]*deliveryPathLock)}

func withDeliveryStoreLock(ctx context.Context, path string, operation func() error) error {
	deliveryPathLocks.Lock()
	lock := deliveryPathLocks.byPath[path]
	if lock == nil {
		lock = &deliveryPathLock{token: make(chan struct{}, 1)}
		lock.token <- struct{}{}
		deliveryPathLocks.byPath[path] = lock
	}
	lock.refs++
	deliveryPathLocks.Unlock()

	select {
	case <-lock.token:
	case <-ctx.Done():
		releaseDeliveryPathLock(path, lock)
		return ctx.Err()
	}
	defer func() {
		lock.token <- struct{}{}
		releaseDeliveryPathLock(path, lock)
	}()
	return withDeliveryStoreFileLock(ctx, path, operation)
}

func releaseDeliveryPathLock(path string, lock *deliveryPathLock) {
	deliveryPathLocks.Lock()
	defer deliveryPathLocks.Unlock()
	lock.refs--
	if lock.refs == 0 {
		delete(deliveryPathLocks.byPath, path)
	}
}
