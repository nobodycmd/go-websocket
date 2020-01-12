package server

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"go-websocket/define"
	"go-websocket/pkg/redis"
	"go-websocket/servers/client"
	"go-websocket/tools/util"
	"log"
	"time"
)

//channel通道
var ToClientChan chan [2]string

// 心跳间隔
var heartbeatInterval = 25 * time.Second

type publishMessage struct {
	GroupName string `json:"groupName"`
	Message   string `json:"message"` //消息内容SendRpcBindGroup
}

func init() {
	ToClientChan = make(chan [2]string, 10)
}

//添加分组到本地
func AddClient2LocalGroup(groupName, clientId *string) {
	if util.IsCluster() {
		groupKey := util.GetGroupKey(*groupName)
		//判断分组是否超过数量限制
		if clientCount, _ := redis.SCARD(groupKey); clientCount >= define.GROUP_CLIENT_LIMIT {
			fmt.Println("客户端数量大于限制")
			//todo 这里需要返回前端错误信息
			return
		}

		//添加客户端ID到集合
		_, err := redis.SetAdd(groupKey, *clientId)
		if err != nil {
			panic(err)
		}
		//记录分组列表
		_, err = redis.SetAdd(define.REDIS_KEY_GROUP_LIST, *groupName)
		if err != nil {
			panic(err)
		}
	}
	client.AddClientToGroup(groupName, clientId)
}

//添加客户端到分组
func AddClient2Group(groupName, clientId *string) {
	//如果是集群则用redis共享数据
	if util.IsCluster() {
		//判断key是否存在
		addr, _, _, isLocal, err := util.GetAddrInfoAndIsLocal(*clientId)
		if err != nil {
			_ = fmt.Errorf("%s", err)
			return
		}

		if isLocal {
			//判断是否已经存在
			if _, isAlive := client.IsAlive(clientId); !isAlive {
				return
			}
			//添加到本地
			AddClient2LocalGroup(groupName, clientId)
		} else {
			//发送到指定的机器
			SendRpcBindGroup(&addr, groupName, clientId)
		}
	} else {
		//判断是否已经存在
		if _, isAlive := client.IsAlive(clientId); !isAlive {
			return
		}
		//如果是单机，就直接添加到本地group了
		AddClient2LocalGroup(groupName, clientId)
	}
}

//获取分组客户端列表
func GetGroupClientList(groupName string) ([]string) {
	if util.IsCluster() {
		groupList, err := redis.SMEMBERS(util.GetGroupKey(groupName))
		if err != nil {
			panic(err)
		}
		return groupList
	}

	return client.GetGroupClientIds(groupName)
}

//发送信息到指定客户端
func SendMessage2Client(clientId, message *string) {
	if util.IsCluster() {
		addr, _, _, isLocal, err := util.GetAddrInfoAndIsLocal(*clientId)
		if err != nil {
			_ = fmt.Errorf("%s", err)
			return
		}

		//如果是本机则发送到本机
		if isLocal {
			go fmt.Println("发送到本机客户端：" + *clientId + " 消息：" + *message)
			SendMessage2LocalClient(clientId, message)
		} else {
			//发送到指定机器
			go fmt.Println("发送到服务器：" + addr + " 客户端：" + *clientId + " 消息：" + *message)
			SendRpc2Client(addr, clientId, message)
		}
	} else {
		//如果是单机服务，则只发送到本机
		SendMessage2LocalClient(clientId, message)
	}
}

//发送到本机分组
func SendMessage2LocalGroup(groupName, message *string) {
	if len(*groupName) > 0 {
		clientList := GetGroupClientList(*groupName)
		if len(clientList) > 0 {
			for _, clientId := range clientList {
				SendMessage2Client(&clientId, message)
			}
		}
	}
}

//发送信息到指定分组
func SendMessage2Group(groupName, message *string) {
	if util.IsCluster() {
		//发送到RabbitMQ
		Send2RabbitMQ(groupName, message)
	} else {
		//如果是单机服务，则只发送到本机
		SendMessage2LocalGroup(groupName, message)
	}
}

//发送到RabbitMQ，方便同步到其他机器
func Send2RabbitMQ(GroupName, message *string) {
	if rabbitMQ == nil {
		panic("rabbitMQ连接失败")
	}

	publishMessage := publishMessage{
		GroupName: *GroupName,
		Message:   *message,
	}

	messageByte, _ := json.Marshal(publishMessage)

	rabbitMQ.PublishPub(string(messageByte))
}

//删除客户端
func DelClient(clientId *string) {
	client.DelClient(clientId)
	if util.IsCluster() {
		//删除redis里的key
		_, _ = redis.Del(define.REDIS_CLIENT_ID_PREFIX + *clientId)

		//获取key所属的分组
		groupList := client.GetClientGroups(clientId)
		for _, groupName := range groupList {
			//删除集群里的分组信息
			_, _ = redis.DelSetKey(util.GetGroupKey(groupName), *clientId)
		}
	}

	//删除客户端里的分组
	client.DelClientGroup(clientId)

	//todo 删除分组里的客户端
}

//通过本服务器发送信息
func SendMessage2LocalClient(clientId, message *string) {
	ToClientChan <- [2]string{*clientId, *message}
}

//监听并发送给客户端信息
func WriteMessage() {
	for {
		select {
		case clientInfo := <-ToClientChan:
			if toConn, ok := client.IsAlive(&clientInfo[0]); ok {
				err := toConn.WriteJSON(clientInfo[1]);
				if err != nil {
					log.Println(err)
				} else {
					//延长key过期时间
					_, err := redis.SetSurvivalTime(define.REDIS_CLIENT_ID_PREFIX+clientInfo[0], define.REDIS_KEY_SURVIVAL_SECONDS)
					if (err != nil) {
						log.Println(err)
					}
				}
			}
		}
	}
}

//启动定时器进行心跳检测
func PingTimer() {
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				//发送心跳
				for clientId, conn := range *client.GetClientList() {
					if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
						_ = conn.Close()
						DelClient(&clientId)
						log.Printf("发送心跳失败: %s 总连接数：%d", clientId, client.ClientNumber())
						return
					}
				}
			}
		}
	}()
}
