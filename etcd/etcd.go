package etcd

import (
	"context"
	"time"

	pb "github.com/coreos/etcd/etcdserver/etcdserverpb"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"go.etcd.io/etcd/clientv3"
)

const (
	EtcdEventTypeUpdate = "update"
	EtcdEventTypeCreate = "create"
	EtcdEventTypeDelete = "delete"
)

const etcdOpTimeout = 5 * time.Second

type EtcdStorage struct {
	id          string
	watchPrefix string
	client      *clientv3.Client
	exitChan    chan struct{}

	watchCtx    context.Context
	watchCancel context.CancelFunc
}

func NewEtcdStorage(id, watchPrefix string, client *clientv3.Client) *EtcdStorage {
	etcd := &EtcdStorage{
		id:          id,
		watchPrefix: watchPrefix,
		client:      client,
		exitChan:    make(chan struct{}),
	}

	return etcd
}

func (etcd *EtcdStorage) WatchPrefix() string {
	return etcd.watchPrefix
}

func (etcd *EtcdStorage) SetWatchPrefix(watchPrefix string) {
	etcd.watchPrefix = watchPrefix
}

func (etcd *EtcdStorage) Close() {
	close(etcd.exitChan)
	etcd.client.Close()
}

func (etcd *EtcdStorage) WatchClose() {
	if etcd.watchCancel != nil {
		etcd.watchCancel()
	}
}

func (etcd *EtcdStorage) Query(key string, withprefix bool, endkey string, limit int64, revision *int64) (*clientv3.GetResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	opts := []clientv3.OpOption{}
	if withprefix {
		opts = []clientv3.OpOption{
			clientv3.WithRange(endkey),
			clientv3.WithKeysOnly(),
			clientv3.WithSerializable(),
			clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		}

		if limit > 0 {
			opts = append(opts, clientv3.WithLimit(limit))
		}
	}

	if revision != nil {
		if *revision >= 0 {
			opts = append(opts, clientv3.WithRev(*revision))
		}
	}

	return etcd.client.Get(ctx, key, opts...)
}

func (etcd *EtcdStorage) Create(key, val string, lease int64) (*clientv3.PutResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	opts := []clientv3.OpOption{
		clientv3.WithPrevKV(),
	}

	if lease > 0 {
		lease, err := etcd.client.Grant(ctx, lease)
		if err != nil {
			return nil, err
		}

		opts = append(opts, clientv3.WithLease(lease.ID))
	}

	return etcd.client.Put(ctx, key, val, opts...)
}

func (etcd *EtcdStorage) Delete(key string, withprefix bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	opts := []clientv3.OpOption{}
	if withprefix {
		opts = append(opts, clientv3.WithPrefix())
	}

	_, err := etcd.client.Delete(ctx, key, opts...)
	return err
}

func (etcd *EtcdStorage) GetRevision(key string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	opts := []clientv3.OpOption{
		clientv3.WithCountOnly(),
	}

	if len(key) == 0 {
		opts = append(opts, clientv3.WithFromKey())
		key = "\x00"
	}

	resp, err := etcd.client.Get(ctx, key, opts...)
	if err != nil {
		return 0, err
	}

	return resp.Header.GetRevision() + 1, nil
}

type EtcdEvent struct {
	ID     string             `json:"-"`
	Type   string             `json:"type"`
	More   bool               `json:"more"`
	Header *pb.ResponseHeader `json:"header"`
	Kvs    []*mvccpb.KeyValue `json:"kvs"`
	PrevKv *mvccpb.KeyValue   `json:"prev_kv,omitempty"`
}

func (etcd *EtcdStorage) Watch(revision int64, key string, c chan *EtcdEvent) {
	opts := []clientv3.OpOption{
		clientv3.WithPrefix(),
		clientv3.WithPrevKV(),
		clientv3.WithRev(revision),
	}

	if len(key) == 0 {
		opts = append(opts, clientv3.WithFromKey())
		key = "\x00"
	}

	for {
		etcd.watchCtx, etcd.watchCancel = context.WithCancel(context.Background())
		watchC := etcd.client.Watch(etcd.watchCtx, key, opts...)
	loop:
		for {
			select {
			case <-etcd.exitChan:
				etcd.watchCancel()
				return
			case resp, ok := <-watchC:
				if resp.Canceled || !ok || resp.Err() != nil {
					etcd.watchCancel()
					time.Sleep(100 * time.Millisecond)

					v, err := etcd.GetRevision(key)
					if err == nil {
						revision = v
					}
					break loop
				}

				for _, event := range resp.Events {
					switch event.Type {
					case clientv3.EventTypeDelete:
						c <- &EtcdEvent{
							ID:     etcd.id,
							Type:   EtcdEventTypeDelete,
							Header: &resp.Header,
							Kvs:    []*mvccpb.KeyValue{event.Kv},
							PrevKv: event.PrevKv,
						}
					case clientv3.EventTypePut:
						typ := EtcdEventTypeUpdate
						if event.IsCreate() {
							typ = EtcdEventTypeCreate
						}
						c <- &EtcdEvent{
							ID:     etcd.id,
							Type:   typ,
							Header: &resp.Header,
							Kvs:    []*mvccpb.KeyValue{event.Kv},
							PrevKv: event.PrevKv,
						}
					}
				}
			}
		}
	}
}
