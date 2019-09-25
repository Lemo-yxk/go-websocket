package lemo

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
)

// Server 服务结构
type Server struct {
	// Host 服务Host
	Host string
	// Port 服务端口
	Port int
	// Path
	Path string
	// Protocol 协议
	Protocol string
	// TLS FILE
	CertFile string
	// TLS KEY
	KeyFile string
}

func (s *Server) CatchError() {
	if err := recover(); err != nil {
		log.Println(string(debug.Stack()), err)
	}
}

// Start 启动 WebSocket
func (s *Server) Start(sh *Socket, hh *Http) {

	var ss = WebSocket(sh)

	// 中间件函数
	var handler = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			defer s.CatchError()

			// var socketPath = r.URL.Path
			// var httpPath = r.URL.Path
			// var serverPath = s.Path

			// Match the websocket router
			if sh.CheckPath(r.URL.Path, s.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Not exists
			if hh == nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write(nil)
				return
			}

			// Get the router
			tire := hh.GetRoute(r.Method, r.URL.Path)
			if tire == nil {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write(nil)
				return
			}

			var hba = tire.Data.(*Hba)

			// Get the middleware
			var context interface{}
			var err error
			var tool Stream
			var params = new(Params)
			params.Keys = tire.Keys
			params.Values = tire.ParseParams(hba.Path)

			tool.rs = rs{w, r, context, params, nil, nil, nil}

			for _, before := range hba.Before {
				context, err = before(&tool)
				if err != nil && hh.OnError != nil {
					hh.OnError(err)
					return
				}
				tool.Context = context
			}

			if hba.StreamFunction != nil {
				err = hba.StreamFunction(&tool)
				if err != nil && hh.OnError != nil {
					hh.OnError(err)
				}
			} else {
				err = hba.HttpFunction(tool.Response, tool.Request)
				if err != nil && hh.OnError != nil {
					hh.OnError(err)
				}
			}

			for _, after := range hba.After {
				err = after(&tool)
				if err != nil && hh.OnError != nil {
					hh.OnError(err)
				}
			}
		})
	}

	s.Run(handler(ss))
}

// Start 启动
func (s *Server) Run(handler http.Handler) {
	if s.Protocol == "TLS" {
		log.Panicln(http.ListenAndServeTLS(fmt.Sprintf("%s:%d", s.Host, s.Port), s.CertFile, s.KeyFile, handler))
	} else {
		log.Panicln(http.ListenAndServe(fmt.Sprintf("%s:%d", s.Host, s.Port), handler))
	}
}
