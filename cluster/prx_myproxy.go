package cluster

import (
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
	"github.com/signal18/replication-manager/router/myproxy"
)

func (cluster *Cluster) initMyProxy(proxy *Proxy) {
	if proxy.InternalProxy != nil {
		proxy.InternalProxy.Close()
	}
	db, err := sql.Open("mysql", cluster.main.DSN)
	if err != nil {
		cluster.LogPrintf(LvlErr, "Could not connect to Main for MyProxy %s", err)
		return
	}
	proxy.InternalProxy, _ = myproxy.NewProxyServer("0.0.0.0:"+proxy.Port, proxy.User, proxy.Pass, db)
	go proxy.InternalProxy.Run()
}
