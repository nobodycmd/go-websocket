package servers

import (
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/woodylan/go-websocket/api"
	"github.com/woodylan/go-websocket/define/retcode"
	"github.com/woodylan/go-websocket/tools/util"
	"net/http"
	"strings"
)

const (
	// 最大的消息大小
	maxMessageSize = 8192

	// Time allowed to write a message to the peer.
	//writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	//pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	//pingPeriod = (pongWait * 9) / 10
)

type Controller struct {
}

type renderData struct {
	ClientId string `json:"clientId"`
}

func (c *Controller) Run(w http.ResponseWriter, r *http.Request) {
	conn, err := (&websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// 是否允许CORS跨域请求
		CheckOrigin: func(r *http.Request) bool {
			refDomain := r.Referer()
			if len(refDomain) > 0 && !strings.Contains(refDomain, r.Host) {
				return false
			}
			return true
		},
	}).Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("upgrade error: %v", err)
		http.NotFound(w, r)
		return
	}

	//设置读取消息大小上线
	conn.SetReadLimit(maxMessageSize)

	//收到pong后
	conn.SetPongHandler(func(appData string) error {
		//fmt.Println("got pong data ", time.Now().String())
		return nil
	})

	//解析参数
	systemId := r.FormValue("systemId")
	if len(systemId) == 0 {
		_ = Render(conn, "", "", retcode.SYSTEM_ID_ERROR, "系统ID不能为空", []string{})
		_ = conn.Close()
		return
	}

	clientId := util.GenClientId()

	clientSocket := NewClient(clientId, systemId, conn)

	Manager.AddClient2SystemClient(systemId, clientSocket)

	//读取客户端消息
	clientSocket.Read()

	if err = api.ConnRender(conn, renderData{ClientId: clientId}); err != nil {
		_ = conn.Close()
		return
	}

	// 用户连接事件
	Manager.Connect <- clientSocket
}
