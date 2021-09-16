// 服务端的基本构建
package main

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type Server struct {
	Ip   string
	Port int

	// 在线用户的列表
	OnlineMap map[string]*User // key用户名 value当前用户对象
	mapLock   sync.RWMutex     // OnlineMap可能是全局的，要加一个锁->这个是互斥锁

	// 互斥锁（sync.Mutex）
	// 互斥锁是一种常用的控制共享资源访问的方法，它能够保证同时只有一个 goroutine 可以访问到共享资源（同一个时刻只有一个线程能够拿到锁）

	// 那么关于锁的使用场景主要涉及到哪些呢？
	// 多个线程在读相同的数据时
	// 多个线程在写相同的数据时
	// 同一个资源，有读又有写

	// 消息广播的channel
	Message chan string
}

// 创建一个server的接口
func NewServer(ip string, port int) *Server {
	server := &Server{
		Ip:        ip,
		Port:      port,
		OnlineMap: make(map[string]*User),
		Message:   make(chan string),
	}

	return server
}

// 监听Message广播消息channel的goroutine, 一旦有消息就发送给全部的在线User
func (this *Server) ListenMessage() {
	for {
		msg := <-this.Message

		//将msg发送给全部的在线User
		this.mapLock.Lock()
		for _, cli := range this.OnlineMap {
			cli.C <- msg
		}
		this.mapLock.Unlock()
	}
}

// 广播消息的方法
func (this *Server) BroadCast(user *User, msg string) {
	sendMsg := "[" + user.Addr + "]" + user.Name + ":" + msg

	this.Message <- sendMsg
}

func (this *Server) Handler(conn net.Conn) {

	// ...当前链接的业务
	user := NewUser(conn, this)

	// 用户的上线业务
	user.Online()

	// 监听用户是否活跃的channel
	isLive := make(chan bool)
	// 接受客户端传递发送的消息
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf) // Read 从连接中读取数据
			if n == 0 {
				user.Offline() // 用户的下线业务

				return
			}

			if err != nil && err != io.EOF {
				fmt.Println("Conn Read err:", err)
				return
			}

			// 提取用户的消息(去除'\n')
			msg := string(buf[:n-1]) // 将字节形式转换成字符串形式

			// 用户针对msg进行消息处理
			user.DoMessage(msg)

			// 用户的任意消息，代表当前用户是一个活跃的
			isLive <- true
		}
	}()

	// 当前handler阻塞
	// 很多时候我们需要让main函数不退出，让它在后台一直执行，例如： select{}
	for {
		select {
		case <-isLive:
			// 当前用户是活跃的， 应该重置定时器
			// 不做任何事情， 为了激活select, 更新下面的定时器
			// isLive 写在 time.After 前面是因为当 isLive被执行时 会尝试 执行之后的case 也就是 time.After(time.Second * 10)
		case <-time.After(time.Second * 300): // 十秒触发， 只有执行这句话就是重置定时器
			// case进来东西的话说明已经超时
			// 将当前的User强制关闭

			user.SendMsg("您被踢了")

			// 销毁用的资源
			close(user.C)

			// 关闭连接
			conn.Close()

			// 退出当前Handler
			return // 也可以用 runtime.Goecit()
		}

	}
}

// 启动服务器的接口
func (this *Server) Start() {

	// socket listen
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", this.Ip, this.Port)) // fmt.Sprintf 拼接字符串
	if err != nil {
		fmt.Println("net.Listen err:", err)
		return
	}
	// socket 的原意是“插座”，在计算机通信领域，socket 被翻译为“套接字”，
	// 它是计算机之间进行通信的一种约定或一种方式。通过 socket 这种约定，
	// 一台计算机可以接收其他计算机的数据，也可以向其他计算机发送数据。
	defer listener.Close() // close listen socket

	// 启动监听Message的goroutine
	go this.ListenMessage()

	for {
		// accept
		conn, err := listener.Accept() // 返回链接的客户端地址
		if err != nil {
			fmt.Println("listener accept err", err)
			continue
		}

		// do handler
		go this.Handler(conn)

	}

}
