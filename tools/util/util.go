package util

import (
	"errors"
	uuid "github.com/satori/go.uuid"
	"github.com/woodylan/go-websocket/pkg/setting"
	"github.com/woodylan/go-websocket/tools/crypto"
	"strings"
)

//GenUUID 生成uuid
func GenUUID() string {
	uuidFunc := uuid.NewV4()
	uuidStr := uuidFunc.String()
	uuidStr = strings.Replace(uuidStr, "-", "", -1)
	return uuidStr
	//uuidByt := []rune(uuidStr)
	//return string(uuidByt[8:24])
}

//对称加密IP和端口，当做clientId
func GenClientId() string {
	raw := []byte(setting.GlobalSetting.LocalHost + ":" + setting.CommonSetting.RPCPort + ":" + GenUUID())
	str, err := crypto.Encrypt(raw, []byte(setting.CommonSetting.CryptoKey))
	if err != nil {
		panic(err)
	}

	return str
}

//得到IP和端口
//sanMaoHaoString 上面22行代码
func GetHostAndPortFromPlainClientIdString(sanMaoHaoString string) (host string, port string, err error) {
	if sanMaoHaoString == "" {
		err = errors.New("解析地址错误")
		return
	}
	addr := strings.Split(sanMaoHaoString, ":")
	if len(addr) < 2 {
		err = errors.New("解析地址错误")
		return
	}
	host, port = addr[0], addr[1]

	return
}

//判断地址是否为本机
func IsAddrLocal(host string, port string) bool {
	return host == setting.GlobalSetting.LocalHost && port == setting.CommonSetting.RPCPort
}

//是否集群
func IsCluster() bool {
	return setting.CommonSetting.Cluster
}

//获取client key地址信息
func GetAddrInfoAndIsLocal(clientId string) (addr string, host string, port string, isLocal bool, err error) {
	//解密ClientId
	addr, err = crypto.Decrypt(clientId, []byte(setting.CommonSetting.CryptoKey))
	if err != nil {
		return
	}

	host, port, err = GetHostAndPortFromPlainClientIdString(addr)
	if err != nil {
		return
	}

	isLocal = IsAddrLocal(host, port)
	return
}

func GenGroupKey(systemId, groupName string) string {
	return systemId + ":" + groupName
}
