package main

import (
	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	"log"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

// 保证金余额：Balance - pnl
// 可转出：Balance - pnl - 持仓金额

// Asset 资产
type Asset struct {
	Balance     decimal.Decimal `json:"balance"`     // 余额：开仓时的余额
	Pnl         decimal.Decimal `json:"pnl"`         // 未实现盈亏
	Margin      decimal.Decimal `json:"margin"`      // 保证金余额：balance-pnl
	CanTransfer decimal.Decimal `json:"canTransfer"` // 可转余额：margin - 持仓金额
}

type ClientManager struct {
	mutex      sync.Mutex
	clients    map[*websocket.Conn]bool
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	broadcast  chan Asset
}

var manager = ClientManager{
	clients:    make(map[*websocket.Conn]bool),
	register:   make(chan *websocket.Conn),
	unregister: make(chan *websocket.Conn),
	broadcast:  make(chan Asset),
	mutex:      sync.Mutex{},
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// 逐循环：注册、注销、广播
func (cm *ClientManager) start() {
	for {
		select {
		case conn := <-cm.register:
			manager.mutex.Lock()
			cm.clients[conn] = true
			manager.mutex.Unlock()
			log.Printf("New client connected. Total clients: %d", len(manager.clients))
		case conn := <-cm.unregister:
			manager.mutex.Lock()
			if _, ok := manager.clients[conn]; ok {
				delete(manager.clients, conn)
				conn.Close()
			}
			log.Printf("Client disconnected. Total clients: %d", len(manager.clients))
		case asset := <-manager.broadcast:
			manager.mutex.Lock()
			for conn := range manager.clients {
				if err := conn.WriteJSON(asset); err != nil {
					log.Printf("write error: %v", err)
					delete(manager.clients, conn)
					conn.Close()
				}
			}
			manager.mutex.Unlock()
		}
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// 读取客户端发送的初始消息（UUID）
	//var uuid string
	//err = conn.ReadJSON(&uuid) // 假设客户端发送的是 JSON 格式的 UUID
	//if err != nil {
	//	log.Printf("error reading UUID: %v", err)
	//	return // 如果读取失败，直接关闭连接
	//}

	manager.register <- conn

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			manager.unregister <- conn
		}
	}
}

func simulatePriceUpdate() {
	previousPrice := 100.0
	balance := 26800.5
	amount := 88.8
	lockBalance := 100.0 * amount
	for {
		// 模拟价格波动
		newPrice := previousPrice * (1 + (rand.Float64()-0.5)/100)
		pnl := (newPrice - previousPrice) * amount
		asset := Asset{
			Balance:     decimal.NewFromFloat(balance),
			Pnl:         decimal.NewFromFloat(pnl),
			Margin:      decimal.NewFromFloat(balance - pnl),
			CanTransfer: decimal.NewFromFloat(balance - pnl - lockBalance),
		}

		manager.broadcast <- asset

		// 模拟每秒更新一次
		time.Sleep(time.Second)
	}
}

func main() {
	go manager.start()
	go simulatePriceUpdate()

	http.HandleFunc("/ws", handleConnections)

	// 启动服务器
	log.Println("Server starting on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

}
