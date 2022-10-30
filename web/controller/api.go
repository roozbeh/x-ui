package controller

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"x-ui/database/model"
	"x-ui/logger"
	"x-ui/web/entity"
	"x-ui/web/global"
	"x-ui/web/service"
)

type APIController struct {
	inboundService service.InboundService
	xrayService    service.XrayService
	userService    service.UserService
}

func NewAPIController(g *gin.RouterGroup) *APIController {
	a := &APIController{}
	a.initRouter(g)
	a.startTask()
	return a
}

func (a *APIController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/api")

	g.GET("/list_users", a.listUsers)
	g.GET("/num_users", a.numUsers)
	g.POST("/add_user", a.addUser)
}

func (a *APIController) startTask() {
	webServer := global.GetWebServer()
	c := webServer.GetCron()
	c.AddFunc("@every 10s", func() {
		if a.xrayService.IsNeedRestartAndSetFalse() {
			err := a.xrayService.RestartXray(false)
			if err != nil {
				logger.Error("restart xray failed:", err)
			}
		}
	})
}

func (a *APIController) numUsers(c *gin.Context) {
	logger.Info("listing users")

	inbounds, err := a.inboundService.GetAllInbounds()
	if err != nil {
		jsonMsg(c, "获取", err)
		return
	}

	m := entity.Msg{
		Obj:     nil,
		Success: true,
		Msg:     strconv.Itoa(len(inbounds)),
	}
	c.JSON(http.StatusOK, m)
}

func (a *APIController) listUsers(c *gin.Context) {
	logger.Info("listing users")

	inbounds, err := a.inboundService.GetAllInbounds()
	if err != nil {
		jsonMsg(c, "获取", err)
		return
	}
	jsonObj(c, inbounds, nil)
}

type VlessObject struct {
	Version string `json:"v"`
	Ps      string `json:"ps"`
	Address string `json:"add"`
	Port    int    `json:"port"`
	UUID    string `json:"id"`
	AlterId int64  `json:"aid"`
	Net     string `json:"net"`
	Type    string `json:"type"`
	Host    string `json:"host"`
	Path    string `json:"path"`
	TLS     string `json:"tls"`
}

func (a *APIController) addUser(c *gin.Context) {
	inbound := &model.Inbound{}
	err := c.ShouldBind(inbound)
	if err != nil {
		jsonMsg(c, "添加", err)
		return
	}
	user, err := a.userService.GetFirstUser()
	if err != nil {
		jsonMsg(c, "添加", err)
		return
	}

	userUUID := uuid.New()
	inbound.Settings = fmt.Sprintf(
		"{\"clients\":[{\"id\":\"%s\",\"flow\":\"xtls-rprx-direct\"}],\"decryption\":\"none\",\"fallbacks\":[]}",
		userUUID)
	userUUIDstring := userUUID.String()

	inbound.StreamSettings = "{\"network\":\"ws\",\"security\":\"none\",\"wsSettings\":{\"acceptProxyProtocol\":false,\"path\":\"/\",\"headers\":{}}}"
	inbound.Sniffing = "{\"enabled\":true,\"destOverride\":[\"http\",\"tls\"]}"

	inbound.Port = 20000 + rand.Intn(30000) /*port between 20,000 to 50,000*/
	inbound.Protocol = "vmess"
	inbound.UserId = user.Id
	inbound.Total = inbound.Total * 1024 * 1024 * 1024
	inbound.Enable = true
	inbound.Tag = fmt.Sprintf("inbound-%v", inbound.Port)
	err = a.inboundService.AddInbound(inbound)

	if err != nil && strings.HasPrefix(err.Error(), "ALREADY_EXISTS") {
		inbound, err = a.inboundService.GetInboundWithRemark(inbound.Remark)

		var settings map[string]any
		json.Unmarshal([]byte(inbound.Settings), &settings)
		clients := settings["clients"].([]any)
		client := clients[0].(map[string]any)
		userUUIDstring = client["id"].(string)
	}

	if err != nil {
		jsonMsg(c, "添加", err)
		return
	}

	res1 := strings.Split(c.Request.Host, ":")
	hostname := res1[0]

	obj := VlessObject{
		Version: "2",
		Ps:      inbound.Remark,
		Address: hostname,
		Port:    inbound.Port,
		UUID:    string(userUUIDstring),
		AlterId: 0,
		Net:     "ws",
		Type:    "none",
		Host:    "",
		Path:    "/",
		TLS:     "none",
	}

	objStr, err := json.Marshal(obj)
	if err != nil {
		jsonMsg(c, "添加", err)
		return
	}

	objStrBase64 := b64.StdEncoding.EncodeToString(objStr)
	vmessURL := "vmess://" + objStrBase64

	m := entity.Msg{
		Obj:     nil,
		Success: true,
		Msg:     vmessURL,
	}
	c.JSON(http.StatusOK, m)

	a.xrayService.SetToNeedRestart()
}
