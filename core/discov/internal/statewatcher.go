//go:generate mockgen -package internal -destination statewatcher_mock.go -source statewatcher.go etcdConn

package internal

import (
	"context"
	"sync"

	"google.golang.org/grpc/connectivity"
)

type (
	etcdConn interface {
		GetState() connectivity.State
		WaitForStateChange(ctx context.Context, sourceState connectivity.State) bool
	}

	stateWatcher struct {
		disconnected bool
		currentState connectivity.State
		listeners    []func()
		// lock only guards listeners, because only listens can be accessed by other goroutines.
		lock sync.Mutex
		// stateListeners is used to notify state change.
		stateListeners []func(up bool)
	}
)

func newStateWatcher() *stateWatcher {
	return new(stateWatcher)
}

func (sw *stateWatcher) addListener(l func()) {
	sw.lock.Lock()
	sw.listeners = append(sw.listeners, l)
	sw.lock.Unlock()
}

func (sw *stateWatcher) notifyListeners() {
	sw.lock.Lock()
	defer sw.lock.Unlock()

	for _, l := range sw.listeners {
		l()
	}
}
func (sw *stateWatcher) addStateListener(l func(bool)) {
	sw.lock.Lock()
	sw.stateListeners = append(sw.stateListeners, l)
	sw.lock.Unlock()
}

func (sw *stateWatcher) notifyStateListeners(stateUp bool) {
	sw.lock.Lock()
	defer sw.lock.Unlock()

	for _, l := range sw.stateListeners {
		l(stateUp)
	}
}

func (sw *stateWatcher) updateState(conn etcdConn) {
	sw.currentState = conn.GetState()
	switch sw.currentState {
	case connectivity.TransientFailure, connectivity.Shutdown:
		if !sw.disconnected {
			sw.notifyStateListeners(false)
		}
		sw.disconnected = true
	case connectivity.Ready:
		if sw.disconnected {
			sw.disconnected = false
			sw.notifyListeners()
			sw.notifyStateListeners(true)
		}
	}
}

func (sw *stateWatcher) watch(conn etcdConn) {
	sw.currentState = conn.GetState()
	sw.notifyStateListeners(sw.currentState == connectivity.Ready)
	for {
		if conn.WaitForStateChange(context.Background(), sw.currentState) {
			sw.updateState(conn)
		}
	}
}
