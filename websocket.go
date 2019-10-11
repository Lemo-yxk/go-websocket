package lemo

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Lemo-yxk/tire"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
)

type Receive struct {
	Context Context
	Params  *Params
	Message *ReceivePackage
}

type ReceivePackage struct {
	MessageType int
	Event       string
	Message     []byte
	ProtoType   int
}

type JsonPackage struct {
	Event   string
	Message interface{}
}

type ProtoBufPackage struct {
	Event   string
	Message proto.Message
}

type PushPackage struct {
	MessageType int
	FD          uint32
	Message     []byte
}

type M map[string]interface{}

// PingMessage PING
const PingMessage int = websocket.PingMessage

// PongMessage PONG
const PongMessage int = websocket.PongMessage

// TextMessage 文本
const TextMessage int = websocket.TextMessage

// BinaryMessage 二进制
const BinaryMessage int = websocket.BinaryMessage

// WebSocket WebSocket
type WebSocket struct {
	Fd       uint32
	Conn     *websocket.Conn
	socket   *WebSocketServer
	Response http.ResponseWriter
	Request  *http.Request
}

// WebSocketServer conn
type WebSocketServer struct {
	fd          uint32
	count       uint32
	connections sync.Map
	OnClose     func(fd uint32)
	OnMessage   func(conn *WebSocket, messageType int, msg []byte)
	OnOpen      func(conn *WebSocket)
	OnError     func(err func() *Error)

	HeartBeatTimeout  int
	HeartBeatInterval int
	HandshakeTimeout  int
	ReadBufferSize    int
	WriteBufferSize   int
	WaitQueueSize     int
	CheckOrigin       func(r *http.Request) bool
	Path              string

	Router *tire.Tire

	IgnoreCase bool

	// 连接
	connOpen chan *WebSocket

	// 关闭
	connClose chan *WebSocket

	// 写入
	connPush chan *PushPackage

	// 返回
	connBack chan error

	upgrade websocket.Upgrader
}

func (socket *WebSocketServer) CheckPath(p1 string, p2 string) bool {
	if socket.IgnoreCase {
		p1 = strings.ToLower(p1)
		p2 = strings.ToLower(p2)
	}
	return p1 == p2
}

func (conn *WebSocket) IP() (string, string, error) {

	if ip := conn.Request.Header.Get(XRealIP); ip != "" {
		return net.SplitHostPort(ip)
	}

	if ip := conn.Request.Header.Get(XForwardedFor); ip != "" {
		return net.SplitHostPort(ip)
	}

	return net.SplitHostPort(conn.Request.RemoteAddr)
}

func (conn *WebSocket) Push(fd uint32, messageType int, msg []byte) error {
	return conn.socket.Push(fd, messageType, msg)
}

func (conn *WebSocket) Json(fd uint32, msg interface{}) error {
	return conn.socket.Json(fd, msg)
}

func (conn *WebSocket) ProtoBuf(fd uint32, msg proto.Message) error {
	return conn.socket.ProtoBuf(fd, msg)
}

func (conn *WebSocket) JsonEmit(fd uint32, msg JsonPackage) error {
	return conn.socket.JsonEmit(fd, msg)
}

func (conn *WebSocket) ProtoBufEmit(fd uint32, msg ProtoBufPackage) error {
	return conn.socket.ProtoBufEmit(fd, msg)
}

func (conn *WebSocket) JsonEmitAll(msg JsonPackage) {
	conn.socket.JsonEmitAll(msg)
}

func (conn *WebSocket) ProtoBufEmitAll(msg ProtoBufPackage) {
	conn.socket.ProtoBufEmitAll(msg)
}

func (conn *WebSocket) GetConnections() chan *WebSocket {
	return conn.socket.GetConnections()
}

func (conn *WebSocket) GetSocket() *WebSocketServer {
	return conn.socket
}

func (conn *WebSocket) GetConnectionsCount() uint32 {
	return conn.socket.GetConnectionsCount()
}

func (conn *WebSocket) GetConnection(fd uint32) (*WebSocket, bool) {
	return conn.socket.GetConnection(fd)
}

// Push 发送消息
func (socket *WebSocketServer) Push(fd uint32, messageType int, msg []byte) error {

	socket.connPush <- &PushPackage{
		MessageType: messageType,
		FD:          fd,
		Message:     msg,
	}

	return <-socket.connBack
}

// Push Json 发送消息
func (socket *WebSocketServer) Json(fd uint32, msg interface{}) error {

	messageJson, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("message error: %v", err)
	}

	return socket.Push(fd, TextMessage, messageJson)
}

func (socket *WebSocketServer) ProtoBuf(fd uint32, msg proto.Message) error {

	messageProtoBuf, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("protobuf error: %v", err)
	}

	return socket.Push(fd, BinaryMessage, messageProtoBuf)
}

func (socket *WebSocketServer) JsonEmitAll(msg JsonPackage) {
	socket.connections.Range(func(key, value interface{}) bool {
		_ = socket.JsonEmit(key.(uint32), msg)
		return true
	})
}

func (socket *WebSocketServer) ProtoBufEmitAll(msg ProtoBufPackage) {
	socket.connections.Range(func(key, value interface{}) bool {
		_ = socket.ProtoBufEmit(key.(uint32), msg)
		return true
	})
}

func (socket *WebSocketServer) ProtoBufEmit(fd uint32, msg ProtoBufPackage) error {

	messageProtoBuf, err := proto.Marshal(msg.Message)
	if err != nil {
		return fmt.Errorf("protobuf error: %v", err)
	}

	return socket.Push(fd, BinaryMessage, Pack([]byte(msg.Event), messageProtoBuf, ProtoBuf, BinaryMessage))

}

func (socket *WebSocketServer) JsonEmit(fd uint32, msg JsonPackage) error {

	var data []byte

	if mb, ok := msg.Message.([]byte); ok {
		data = mb
	} else {
		messageJson, err := json.Marshal(msg.Message)
		if err != nil {
			return fmt.Errorf("protobuf error: %v", err)
		}
		data = messageJson
	}

	return socket.Push(fd, TextMessage, Pack([]byte(msg.Event), data, Json, TextMessage))

}

func (socket *WebSocketServer) addConnect(conn *WebSocket) {

	// +1
	socket.fd++

	// 溢出
	if socket.fd == 0 {
		socket.fd++
	}

	var _, ok = socket.connections.Load(socket.fd)

	if !ok {
		socket.connections.Store(socket.fd, conn)
	} else {
		// 否则查找最大值
		var maxFd uint32 = 0

		for {

			maxFd++

			if maxFd == 0 {
				println("connections overflow")
				return
			}

			var _, ok = socket.connections.Load(socket.fd)

			if !ok {
				socket.connections.Store(maxFd, conn)
				break
			}

		}

		socket.fd = maxFd
	}

	// 赋值
	conn.Fd = socket.fd

}

func (socket *WebSocketServer) delConnect(conn *WebSocket) {
	socket.connections.Delete(conn.Fd)
}

func (socket *WebSocketServer) GetConnections() chan *WebSocket {

	var ch = make(chan *WebSocket, 1024)

	go func() {
		socket.connections.Range(func(key, value interface{}) bool {
			ch <- value.(*WebSocket)
			return true
		})
		close(ch)
	}()

	return ch
}

func (socket *WebSocketServer) GetConnection(fd uint32) (*WebSocket, bool) {
	conn, ok := socket.connections.Load(fd)
	if !ok {
		return nil, false
	}
	return conn.(*WebSocket), true
}

func (socket *WebSocketServer) GetConnectionsCount() uint32 {
	return socket.count
}

func (socket *WebSocketServer) Init() {

	if socket.HeartBeatTimeout == 0 {
		socket.HeartBeatTimeout = 30
	}

	if socket.HeartBeatInterval == 0 {
		socket.HeartBeatInterval = 15
	}

	if socket.HandshakeTimeout == 0 {
		socket.HandshakeTimeout = 2
	}

	// must be 4096 or the memory will leak
	if socket.ReadBufferSize == 0 {
		socket.ReadBufferSize = 4096
	}
	// must be 4096 or the memory will leak
	if socket.WriteBufferSize == 0 {
		socket.WriteBufferSize = 4096
	}

	if socket.WaitQueueSize == 0 {
		socket.WaitQueueSize = 1024
	}

	if socket.CheckOrigin == nil {
		socket.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	}

	if socket.OnOpen == nil {
		socket.OnOpen = func(conn *WebSocket) {
			println(conn.Fd, "is open")
		}
	}

	if socket.OnClose == nil {
		socket.OnClose = func(fd uint32) {
			println(fd, "is close")
		}
	}

	if socket.OnError == nil {
		socket.OnError = func(err func() *Error) {
			println(err())
		}
	}

	socket.upgrade = websocket.Upgrader{
		HandshakeTimeout: time.Duration(socket.HandshakeTimeout) * time.Second,
		ReadBufferSize:   socket.ReadBufferSize,
		WriteBufferSize:  socket.WriteBufferSize,
		CheckOrigin:      socket.CheckOrigin,
	}

	// 连接
	socket.connOpen = make(chan *WebSocket, socket.WaitQueueSize)

	// 关闭
	socket.connClose = make(chan *WebSocket, socket.WaitQueueSize)

	// 写入
	socket.connPush = make(chan *PushPackage, socket.WaitQueueSize)

	// 返回
	socket.connBack = make(chan error, socket.WaitQueueSize)

	go func() {
		for {
			select {
			case conn := <-socket.connOpen:
				socket.addConnect(conn)
				socket.count++
				// 触发OPEN事件
				go socket.OnOpen(conn)
			case conn := <-socket.connClose:
				var fd = conn.Fd
				socket.delConnect(conn)
				socket.count--
				// 触发CLOSE事件
				go socket.OnClose(fd)
			case push := <-socket.connPush:
				var conn, ok = socket.connections.Load(push.FD)
				if !ok {
					socket.connBack <- fmt.Errorf("client %d is close", push.FD)
				} else {
					socket.connBack <- conn.(*WebSocket).Conn.WriteMessage(push.MessageType, push.Message)
				}
			}
		}
	}()

}

func (socket *WebSocketServer) handler(w http.ResponseWriter, r *http.Request) {

	// 升级协议
	conn, err := socket.upgrade.Upgrade(w, r, nil)

	// 错误处理
	if err != nil {
		go socket.OnError(NewError(err))
		return
	}

	// 超时时间
	err = conn.SetReadDeadline(time.Now().Add(time.Duration(socket.HeartBeatTimeout) * time.Second))
	if err != nil {
		go socket.OnError(NewError(err))
		return
	}

	connection := &WebSocket{
		Fd:       0,
		Conn:     conn,
		socket:   socket,
		Response: w,
		Request:  r,
	}

	// 设置PING处理函数
	conn.SetPingHandler(func(appData string) error {
		// unnecessary
		// err := socket.Push(connection.Fd, PongMessage, nil)
		return conn.SetReadDeadline(time.Now().Add(time.Duration(socket.HeartBeatTimeout) * time.Second))
	})

	// 设置PONG处理函数
	conn.SetPongHandler(func(appData string) error {
		return nil
	})

	// 打开连接 记录
	socket.connOpen <- connection

	// 收到消息 处理 单一连接接受不冲突 但是不能并发写入
	for {

		// read message
		frameType, message, err := conn.ReadMessage()
		// close
		if err != nil {
			break
		}

		// unpack
		version, messageType, protoType, route, body := UnPack(message)

		// check version
		if version != Version {
			if socket.OnMessage != nil {
				go socket.OnMessage(connection, messageType, message)
			}
			continue
		}

		// check message type
		if frameType != messageType {
			break
		}

		// Ping
		if messageType == PingMessage {
			err := conn.PingHandler()("")
			if err != nil {
				break
			}
			continue
		}

		// Pong
		if messageType == PongMessage {
			err := conn.PongHandler()("")
			if err != nil {
				break
			}
			continue
		}

		// on router
		if socket.Router != nil {
			var receivePackage = &ReceivePackage{MessageType: messageType, Event: string(route), Message: body, ProtoType: protoType}
			go socket.router(connection, receivePackage)
			continue
		}

	}

	// close and clean
	_ = conn.Close()
	socket.connClose <- connection

}
