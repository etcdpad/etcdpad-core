package httphandle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/etcdpad/etcdpad-core/etcd"

	"go.etcd.io/etcd/clientv3"
	"go.uber.org/zap"
)

type EtcdHandle struct {
	lg *zap.Logger

	etcdStores map[string]*etcd.EtcdStorage
	connMap    map[string][]*websocketClient

	etchChangeC chan *etcd.EtcdEvent

	lock     sync.RWMutex
	wg       sync.WaitGroup
	exitChan chan struct{}
}

func NewEtcdHandle(lg *zap.Logger) *EtcdHandle {
	eh := &EtcdHandle{
		lg:          lg,
		etcdStores:  make(map[string]*etcd.EtcdStorage),
		etchChangeC: make(chan *etcd.EtcdEvent),
		connMap:     make(map[string][]*websocketClient),
		exitChan:    make(chan struct{}),
	}

	go eh.handleEtcdEvent()

	return eh
}

func (eh *EtcdHandle) handleEtcdEvent() {
	eh.wg.Add(1)
	for {
		select {
		case <-eh.exitChan:
			goto exit
		case event := <-eh.etchChangeC:
			eh.lock.RLock()
			wc := make([]*websocketClient, len(eh.connMap[event.ID]))
			copy(wc, eh.connMap[event.ID])
			eh.lock.RUnlock()

			key := string(event.Kvs[0].Key)
			if event.Type == etcd.EtcdEventTypeDelete {
				key = string(event.PrevKv.Key)
			}

			cresp := CommunicateResponse{
				Success: true,
				Action:  clientOpTypeWatch,
				Key:     key,
				Event:   event,
			}

			b := cresp.Bytes()
			for _, c := range wc {
				if strings.HasPrefix(key, c.etcdprefix) {
					select {
					case c.message <- b:
					case <-eh.exitChan:
						goto exit
					}
				}
			}
		}
	}
exit:
	eh.wg.Done()
}

func (eh *EtcdHandle) Close() {
	close(eh.exitChan)
	eh.wg.Wait()

	eh.lock.Lock()
	for _, conns := range eh.connMap {
		for _, conn := range conns {
			conn.once.Do(conn.exit)
		}
	}
	for _, etcdStore := range eh.etcdStores {
		etcdStore.Close()
	}

	eh.etcdStores = make(map[string]*etcd.EtcdStorage)
	eh.connMap = make(map[string][]*websocketClient)
	eh.lock.Unlock()
}

func (eh *EtcdHandle) closeWs(key, uuid string) {
	eh.lock.Lock()
	defer eh.lock.Unlock()

	conns := eh.connMap[key]

	index := -1
	for i, conn := range conns {
		if conn.uuid == uuid {
			index = i
			break
		}
	}

	if index >= 0 {
		if len(conns) == 1 {
			conns[index] = nil
			delete(eh.connMap, key)
			etcdStore := eh.etcdStores[key]
			if etcdStore != nil {
				etcdStore.Close()
			}
			delete(eh.etcdStores, key)
			return
		}
		conns[index], conns[len(conns)-1] = conns[len(conns)-1], conns[index]
		conns[len(conns)-1] = nil
		conns = conns[:len(conns)-1]
		eh.connMap[key] = conns
	}
}

func (eh *EtcdHandle) addEtcdClient(client *websocketClient) error {
	eh.lock.Lock()
	defer eh.lock.Unlock()

	var etcdStore *etcd.EtcdStorage
	var ok bool

	etcdStore, ok = eh.etcdStores[client.key]
	if !ok {
		etcdClient, err := clientv3.New(client.config)
		if err != nil {
			return err
		}

		etcdStore = etcd.NewEtcdStorage(client.key, client.etcdprefix, etcdClient)
		eh.etcdStores[client.key] = etcdStore

		revision, err := etcdStore.GetRevision(client.etcdprefix)
		if err != nil {
			return err
		}

		go etcdStore.Watch(context.Background(), revision, client.etcdprefix, eh.etchChangeC)
	} else {
		// current etcd watch prefix key need modify
		// for example
		// current watch prefix: /foo/bar
		// new client watch prefix: /foo
		if strings.HasPrefix(etcdStore.WatchPrefix(), client.etcdprefix) {
			etcdStore.WatchClose()
			etcdStore.SetWatchPrefix(client.etcdprefix)
			revision, err := etcdStore.GetRevision(client.etcdprefix)
			if err != nil {
				return err
			}
			go etcdStore.Watch(context.Background(), revision, client.etcdprefix, eh.etchChangeC)
		} else {
			if !strings.HasPrefix(client.etcdprefix, etcdStore.WatchPrefix()) {
				return fmt.Errorf("watch prefix [%s] must be from /", client.etcdprefix)
			}
		}
	}

	client.etcdStore = etcdStore

	_, ok = eh.connMap[client.key]
	if ok {
		eh.connMap[client.key] = append(eh.connMap[client.key], client)
	} else {
		eh.connMap[client.key] = []*websocketClient{client}
	}

	return nil
}

func decode(username, password string, endpoints []string) string {
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i] < endpoints[j]
	})

	cha := username + password + strings.Join(endpoints, ",")
	h := sha256.New()
	h.Write([]byte(cha))
	return hex.EncodeToString(h.Sum(nil))
}
