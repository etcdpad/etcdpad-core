package engine

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/etcdpad/etcdpad-core/httphandle"
	"github.com/etcdpad/etcdpad-core/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type WebEngine struct {
	prefix string

	lg     *zap.Logger
	server *http.Server
	router *gin.Engine

	handle *httphandle.EtcdHandle
}

func NewEngine(port int, logpath string, stdout bool) *WebEngine {

	lg := logger.NewLogger(logpath, zap.InfoLevel, stdout)

	engine := &WebEngine{
		prefix: "/epad",
		lg:     lg,
		router: gin.New(),
		handle: httphandle.NewEtcdHandle(lg),
	}

	engine.registerRouter()

	engine.server = &http.Server{
		Addr:         fmt.Sprintf("0.0.0.0:%d", port),
		Handler:      engine.router,
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 40 * time.Second,
	}

	go func() {
		if err := engine.server.ListenAndServe(); err != nil {
			log.Fatal("etcdpad listenAndServe error ", err.Error())
		}
	}()

	return engine
}

func (engine *WebEngine) ShutDown() {
	if engine.server != nil {
		if err := engine.server.Shutdown(context.Background()); err != nil {
			fmt.Println("Server Shutdown: ", err)
		}
	}

	engine.lg.Sync()
	engine.handle.Close()
}
