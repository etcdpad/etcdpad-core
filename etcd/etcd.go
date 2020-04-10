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
}

func NewEtcdStorage(id, watchPrefix string, client *clientv3.Client) *EtcdStorage {
	return &EtcdStorage{
		id:          id,
		watchPrefix: watchPrefix,
		client:      client,
		exitChan:    make(chan struct{}),
	}
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
	etcd.client.Watcher.Close()
}

func (etcd *EtcdStorage) Query(key string, withprefix bool, limit int64) (*clientv3.GetResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	opts := []clientv3.OpOption{}
	if withprefix {
		endKey := []byte("\x00")
		if len(key) > 0 {
			endKey = []byte(key)
			endKey[len(endKey)-1]++
		}

		opts = []clientv3.OpOption{
			clientv3.WithPrefix(),
			clientv3.WithRange(string(endKey)),
			//clientv3.WithFromKey(),
			clientv3.WithKeysOnly(),
			clientv3.WithSerializable(),
			clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
		}

		if limit > 0 {
			opts = append(opts, clientv3.WithLimit(limit))
		}
	} else {
		opts = append(opts, clientv3.WithPrevKV())
	}

	return etcd.client.Get(ctx, key, opts...)
}

func (etcd *EtcdStorage) Create(key, val string, lease int64) (*clientv3.PutResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), etcdOpTimeout)
	defer cancel()

	if lease > 0 {
		lease, err := etcd.client.Grant(ctx, lease)
		if err != nil {
			return nil, err
		}
		return etcd.client.Put(ctx, key, val, clientv3.WithLease(lease.ID))
	}

	return etcd.client.Put(ctx, key, val)
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

	resp, err := etcd.client.Get(ctx, key, clientv3.WithCountOnly())
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
	Kvs    []*mvccpb.KeyValue `json:"kvs,omitempty"`
	PrevKv *mvccpb.KeyValue   `json:"prev_kv,omitempty"`
}

func (etcd *EtcdStorage) Watch(ctx context.Context, revision int64, key string, c chan *EtcdEvent) {
	watchC := etcd.client.Watch(ctx, key, clientv3.WithPrefix(), clientv3.WithPrevKV(), clientv3.WithRev(revision))

	for {
		select {
		case <-ctx.Done():
			return
		case <-etcd.exitChan:
			return
		case resp := <-watchC:
			if resp.Canceled {
				return
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
