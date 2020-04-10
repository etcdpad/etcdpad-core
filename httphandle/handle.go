package httphandle

import (
	"net/http"
	"net/url"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  0,
	WriteBufferSize: 0,
	WriteBufferPool: &sync.Pool{},
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (eh *EtcdHandle) Connect(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	cr := &CommunicateResponse{
		Action:  clientOpTypeHandShake,
		Success: true,
	}

	dsn, err := url.QueryUnescape(c.Query("dsn"))
	if err != nil {
		handleErr(conn, cr, err)
		return
	}
	etcdprefix, config, err := parseURL(dsn)
	if err != nil {
		handleErr(conn, cr, err)
		return
	}

	client := newWebsocketClient(eh.lg, c.ClientIP(), etcdprefix, config, conn, eh)

	err = eh.addEtcdClient(client)
	if err != nil {
		handleErr(conn, cr, err)
		return
	}

	// handshake success
	cr.ID = client.uuid
	conn.WriteMessage(websocket.TextMessage, cr.Bytes())

	go client.read()
	go client.write()
}

func handleErr(conn *websocket.Conn, cr *CommunicateResponse, err error) {
	cr.Err = err.Error()
	cr.Success = false
	conn.WriteMessage(websocket.TextMessage, cr.Bytes())
	conn.Close()
}
