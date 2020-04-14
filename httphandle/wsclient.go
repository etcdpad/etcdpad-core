package httphandle

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/etcdpad/etcdpad-core/etcd"

	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
	"github.com/xkeyideal/gokit/tools"
	"go.etcd.io/etcd/clientv3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	clientOpTypeCreate    = "create"
	clientOpTypeDelete    = "delete"
	clientOpTypeQuery     = "query"
	clientOpTypeWatch     = "watch"
	clientOpTypePing      = "ping"
	clientOpTypePong      = "pong"
	clientOpTypeHandShake = "handshake"
)

type websocketClient struct {
	owner     *EtcdHandle
	etcdStore *etcd.EtcdStorage

	uuid       string
	key        string
	conn       *websocket.Conn
	lastActive time.Time

	logger *zap.Logger

	etcdprefix string
	config     clientv3.Config

	remoteIP string

	message      chan []byte
	communicates chan *CommunicateMessage

	wg       sync.WaitGroup
	once     sync.Once
	exitChan chan struct{}
}

type CommunicateMessage struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Key    string `json:"key"`
	Value  string `json:"val"`
	Lease  int64  `json:"lease"`
	Limit  int64  `json:"limit"`
	Prefix bool   `json:"prefix"`
	EndKey string `json:"endkey"`

	// Revision is a global revision. ModRevision is the revision that the key is modified.
	// the Revision is the current revision of etcd.
	// It is incremented every time the v3 backed is modified (e.g., Put, Delete, Txn).
	// ModRevision is the etcd revision of the last update to a key.
	// Version is the number of times the key has been modified since it was created.
	// Get(..., WithRev(rev)) will perform a Get as if the etcd store is still at revision rev
	// CreateRevision is the revision of last creation on this key.
	Revision *int64 `json:"revision"`
}

func (cm *CommunicateMessage) String() string {
	b, _ := json.Marshal(cm)
	return string(b)
}

type CommunicateResponse struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Success bool            `json:"success"`
	Key     string          `json:"key"`
	Event   *etcd.EtcdEvent `json:"event"`
	Err     string          `json:"err"`
}

func (cr *CommunicateResponse) String() string {
	b, _ := json.Marshal(cr)
	return string(b)
}

func (cr *CommunicateResponse) Bytes() []byte {
	b, _ := json.Marshal(cr)
	return b
}

func newWebsocketClient(lg *zap.Logger,
	remoteIP, etcdprefix string,
	config clientv3.Config, conn *websocket.Conn,
	owner *EtcdHandle) *websocketClient {

	return &websocketClient{
		owner: owner,
		uuid:  uuid.NewV4().String(),
		key:   decode(config.Username, config.Password, config.Endpoints),
		conn:  conn,

		remoteIP:   remoteIP,
		etcdprefix: etcdprefix,
		config:     config,
		lastActive: tools.CSTNow(),
		logger:     lg,

		message:      make(chan []byte),
		communicates: make(chan *CommunicateMessage),
		exitChan:     make(chan struct{}),
	}
}

func (client *websocketClient) exit() {
	close(client.exitChan)
	client.wg.Wait()
	client.conn.Close()
}

func (client *websocketClient) close() {
	close(client.exitChan)
	client.wg.Wait()
	client.conn.Close()
	client.owner.closeWs(client.key, client.uuid)
}

func (client *websocketClient) read() {
	client.conn.SetPongHandler(func(s string) error {
		client.lastActive = tools.CSTNow()
		return nil
	})
	client.conn.SetPingHandler(func(s string) error {
		client.lastActive = tools.CSTNow()
		return nil
	})

	client.wg.Add(1)
	for {
		select {
		case <-client.exitChan:
			goto exit
		default:
			_, message, err := client.conn.ReadMessage()
			if err != nil {
				client.wg.Done()
				client.once.Do(client.close)
				return
			}

			client.lastActive = tools.CSTNow()

			cm := &CommunicateMessage{}
			err = json.Unmarshal(message, cm)
			if err != nil {
				continue
			}

			select {
			case client.communicates <- cm:
			case <-client.exitChan:
				goto exit
			}
		}
	}
exit:
	client.wg.Done()
}

func (client *websocketClient) write() {
	ticker := time.NewTicker(5 * time.Second)

	client.wg.Add(1)
	for {
		select {
		case <-client.exitChan:
			goto exit
		case message := <-client.message:
			client.conn.WriteMessage(websocket.TextMessage, message)
		case cm := <-client.communicates:
			cresp := &CommunicateResponse{
				ID:      cm.ID,
				Action:  cm.Action,
				Key:     cm.Key,
				Success: false,
			}

			switch cm.Action {
			case clientOpTypeCreate:
				resp, err := client.etcdStore.Create(cm.Key, cm.Value, cm.Lease)
				if err != nil {
					cresp.Err = err.Error()
				} else {
					cresp.Success = true
					cresp.Event = &etcd.EtcdEvent{
						Header: resp.Header,
						PrevKv: resp.PrevKv,
					}
				}
				client.logger.Info(fmt.Sprintf("[%s] create", client.uuid),
					zap.Bool("success", cresp.Success),
					zap.String("remote_ip", client.remoteIP),
					zap.Strings("endpoints", client.config.Endpoints),
					zap.Stringer("op", cm),
					zap.Stringer("resp", cresp),
				)
			case clientOpTypeDelete:
				fields := []zapcore.Field{
					zap.String("remote_ip", client.remoteIP),
					zap.Strings("endpoints", client.config.Endpoints),
					zap.Stringer("op", cm),
				}

				err := client.etcdStore.Delete(cm.Key, cm.Prefix)
				if err != nil {
					cresp.Err = err.Error()
					fields = append(fields, zap.String("err", err.Error()))
				} else {
					cresp.Success = true
				}

				fields = append(fields, zap.Bool("success", cresp.Success))
				client.logger.Warn(fmt.Sprintf("[%s] delete", client.uuid), fields...)
			case clientOpTypeQuery:
				resp, err := client.etcdStore.Query(cm.Key, cm.Prefix, cm.EndKey, cm.Limit, cm.Revision)
				if err != nil {
					cresp.Err = err.Error()
				} else {
					cresp.Success = true
					cresp.Event = &etcd.EtcdEvent{
						More:   resp.More,
						Header: resp.Header,
						Kvs:    []*mvccpb.KeyValue{},
					}
					if len(resp.Kvs) > 0 {
						cresp.Event.Kvs = resp.Kvs
					}
				}
			case clientOpTypePing:
				cresp.Action = clientOpTypePong
				cresp.Success = true
			}

			client.conn.WriteMessage(websocket.TextMessage, cresp.Bytes())
		case <-ticker.C:
			if tools.CSTNow().Sub(client.lastActive) > 120*time.Second {
				client.wg.Done()
				client.once.Do(client.close)
				return
			}
		}
	}
exit:
	client.wg.Done()
}
