package lobbyrpc

import (
	"net"

	"github.com/valyala/gorpc"
)

func init() {
	gorpc.RegisterType(new(RegisterProxyRequest))
	gorpc.RegisterType(new(RegisterProxyResponse))
	gorpc.RegisterType(new(BattleInfoRequest))
	gorpc.RegisterType(new(BattleInfoResponse))
	gorpc.RegisterType(new(StatusRequest))
	gorpc.RegisterType(new(StatusResponse))
}

type RegisterProxyRequest struct {
	CurrentVersion int
	UserID         string
	LocalIP        net.IP
	Port           uint16
	UDPAddrs       []string
	P2PConnected   map[string]struct{}
}

type RegisterProxyResponse struct {
	Result     bool
	Message    string
	SessionID  string
	UserID     string
	LobbyUsers []User
}

type BattleInfoRequest struct {
	SessionID string
}

type BattleInfoResponse struct {
	Result   bool
	Message  string
	Users    []User
	BattleIP net.IP
	Port     uint16
	IsTest   bool
}

type StatusRequest struct {
}

type User struct {
	UserID   string
	Name     string
	Team     string
	UDP      bool
	UDPAddrs []string
}

type Battle struct {
	Users     []User
	AeugIDs   []string
	TitansIDs []string
}

type StatusResponse struct {
	LobbyUsers []User
	Battles    []Battle
}
