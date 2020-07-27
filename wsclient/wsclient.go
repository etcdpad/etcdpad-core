package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/etcdpad/etcdpad-core/etcd"
	"github.com/etcdpad/etcdpad-core/httphandle"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Engine struct {
	server *http.Server
	router *gin.Engine

	wsconn *websocket.Conn
}

type EtcdOp struct {
	Op  string `json:"op"`
	Key string `json:"key"`
	Val string `json:"val"`
}

func (engine *Engine) test(c *gin.Context) {
	eop := EtcdOp{}

	bytes, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"msg": err.Error(),
		})
		return
	}
	if err := json.Unmarshal(bytes, &eop); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"msg": err.Error(),
		})
		return
	}

	msg := httphandle.CommunicateMessage{
		ID:     "xx",
		Action: eop.Op,
		Key:    eop.Key,
		Value:  eop.Val,
	}

	b, _ := json.Marshal(msg)

	engine.wsconn.WriteMessage(websocket.TextMessage, b)

	c.JSON(http.StatusOK, gin.H{
		"msg": "OK",
	})
}

func (engine *Engine) registerRouter() {
	group := engine.router.Group("/test")
	{
		group.POST("/set", engine.test)
	}
}

func newEngine() *Engine {
	engine := &Engine{
		router: gin.New(),
	}

	engine.registerRouter()

	engine.server = &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", 12346),
		Handler:      engine.router,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 40 * time.Second,
	}

	u := url.URL{Scheme: "ws", Host: "127.0.0.1:8989", Path: "/epad/ws", RawQuery: "dsn=root:root@10.100.47.3:6005"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}

	engine.wsconn = c

	go func() {
		if err := engine.server.ListenAndServe(); err != nil {
			log.Fatal("etcdpad listenAndServe error ", err.Error())
		}
	}()

	go func() {
		for {
			_, message, err := engine.wsconn.ReadMessage()
			if err != nil {
				log.Panicln(err)
			}

			resp := httphandle.CommunicateResponse{
				Event: &etcd.EtcdEvent{},
			}
			json.Unmarshal(message, &resp)
			if resp.Key == "/etcdpad/test" {
				fmt.Println(string(message))
				//log.Println("watch:", resp.Action, resp.Event.Kvs[0])
			}
		}
	}()

	return engine
}

func (engine *Engine) Stop() {
	engine.server.Shutdown(context.Background())
	engine.wsconn.Close()
}

func main() {
	eg := newEngine()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	<-signals

	eg.Stop()
}
