// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.
// Redistribution/Reuse of this code is permitted under the GNU v3 license, as
// an additional term, ALL code must carry the original Author(s) credit in comment form.
// See LICENSE in this directory for the integral text.
package cluster

import (
	"fmt"
	"strconv"

	"github.com/signal18/replication-manager/router/maxscale"
	"github.com/signal18/replication-manager/utils/state"
)

func (cluster *Cluster) refreshMaxscale(proxy *Proxy) error {
	if cluster.Conf.MxsOn == false {
		return nil
	}
	var m maxscale.MaxScale
	if proxy.Tunnel {
		m = maxscale.MaxScale{Host: "localhost", Port: strconv.Itoa(proxy.TunnelPort), User: proxy.User, Pass: proxy.Pass}
	} else {
		m = maxscale.MaxScale{Host: proxy.Host, Port: proxy.Port, User: proxy.User, Pass: proxy.Pass}
	}

	if cluster.Conf.MxsOn {
		err := m.Connect()
		if err != nil {
			cluster.sme.AddState("ERR00018", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00018"], err), ErrFrom: "CONF"})
			cluster.sme.CopyOldStateFromUnknowServer(proxy.Name)
			return err
		}
	}
	proxy.BackendsWrite = nil
	for _, server := range cluster.Servers {

		var bke = Backend{
			Host:    server.Host,
			Port:    server.Port,
			Status:  server.State,
			PrxName: server.URL,
		}

		if cluster.Conf.MxsGetInfoMethod == "maxinfo" {
			_, err := m.GetMaxInfoServers("http://" + proxy.Host + ":" + strconv.Itoa(cluster.Conf.MxsMaxinfoPort) + "/servers")
			if err != nil {
				cluster.sme.AddState("ERR00020", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00020"], server.URL), ErrFrom: "MON", ServerUrl: proxy.Name})
			}
			srvport, _ := strconv.Atoi(server.Port)
			mxsConnections := 0
			bke.PrxName, bke.PrxStatus, mxsConnections = m.GetMaxInfoServer(server.Host, srvport, server.ClusterGroup.Conf.MxsServerMatchPort)
			bke.PrxConnections = strconv.Itoa(mxsConnections)
			server.MxsServerStatus = bke.PrxStatus
			server.MxsServerName = bke.PrxName

		} else {
			_, err := m.ListServers()
			if err != nil {
				server.ClusterGroup.sme.AddState("ERR00019", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00019"], server.URL), ErrFrom: "MON", ServerUrl: proxy.Name})
			} else {

				if proxy.Tunnel {

					bke.PrxName, bke.PrxStatus, bke.PrxConnections = m.GetServer(server.Host, server.Port, server.ClusterGroup.Conf.MxsServerMatchPort)
					server.MxsServerStatus = bke.PrxStatus
					server.MxsServerName = bke.PrxName

				} else {
					bke.PrxName, bke.PrxStatus, bke.PrxConnections = m.GetServer(server.Host, server.Port, server.ClusterGroup.Conf.MxsServerMatchPort)
					server.MxsServerStatus = bke.PrxStatus
					server.MxsServerName = bke.PrxName
				}
				//server.ClusterGroup.LogPrintf("INFO", "Affect for server %s, %s %s  ", server.IP, server.MxsServerName, server.MxsServerStatus)
			}
		}
		proxy.BackendsWrite = append(proxy.BackendsWrite, bke)
	}
	m.Close()
	return nil
}

func (cluster *Cluster) initMaxscale(oldmain *ServerMonitor, proxy *Proxy) {
	if cluster.Conf.MxsOn == false {
		return
	}

	var m maxscale.MaxScale
	if proxy.Tunnel {
		m = maxscale.MaxScale{Host: "localhost", Port: strconv.Itoa(proxy.TunnelPort), User: proxy.User, Pass: proxy.Pass}
	} else {
		m = maxscale.MaxScale{Host: proxy.Host, Port: proxy.Port, User: proxy.User, Pass: proxy.Pass}
	}
	err := m.Connect()
	if err != nil {
		cluster.LogPrintf(LvlErr, "Could not connect to MaxScale:%s", err)
		return
	}
	defer m.Close()
	if cluster.GetMain().MxsServerName == "" {
		return
	}

	var monitor string
	if cluster.Conf.MxsGetInfoMethod == "maxinfo" {
		cluster.LogPrintf(LvlDbg, "Getting Maxscale monitor via maxinfo")
		m.GetMaxInfoMonitors("http://" + cluster.Conf.MxsHost + ":" + strconv.Itoa(cluster.Conf.MxsMaxinfoPort) + "/monitors")
		monitor = m.GetMaxInfoMonitor()

	} else {
		cluster.LogPrintf(LvlDbg, "Getting Maxscale monitor via maxadmin")
		_, err := m.ListMonitors()
		if err != nil {
			cluster.LogPrintf(LvlErr, "MaxScale client could not list monitors %s", err)
		}
		monitor = m.GetMonitor()
	}
	if monitor != "" && cluster.Conf.MxsDisableMonitor == true {
		cmd := "shutdown monitor \"" + monitor + "\""
		cluster.LogPrintf(LvlInfo, "Maxscale shutdown monitor: %s", cmd)
		err = m.ShutdownMonitor(monitor)
		if err != nil {
			cluster.LogPrintf(LvlErr, "MaxScale client could not shutdown monitor:%s", err)
		}
		m.Response()
		if err != nil {
			cluster.LogPrintf(LvlErr, "MaxScale client could not shutdown monitor:%s", err)
		}
	} else {
		cluster.sme.AddState("ERR00017", state.State{ErrType: "ERROR", ErrDesc: clusterError["ERR00017"], ErrFrom: "TOPO", ServerUrl: proxy.Name})
	}

	err = m.SetServer(cluster.GetMain().MxsServerName, "main")
	if err != nil {
		cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
	}
	err = m.SetServer(cluster.GetMain().MxsServerName, "running")
	if err != nil {
		cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
	}
	err = m.ClearServer(cluster.GetMain().MxsServerName, "subordinate")
	if err != nil {
		cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
	}

	if cluster.Conf.MxsBinlogOn == false {
		for _, s := range cluster.Servers {
			if s != cluster.GetMain() {

				err = m.ClearServer(s.MxsServerName, "main")
				if err != nil {
					cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
				}

				if s.State != stateSubordinate {
					err = m.ClearServer(s.MxsServerName, "subordinate")
					if err != nil {
						cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
					}
					err = m.ClearServer(s.MxsServerName, "running")
					if err != nil {
						cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
					}

				} else {
					err = m.SetServer(s.MxsServerName, "subordinate")
					if err != nil {
						cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
					}
					err = m.SetServer(s.MxsServerName, "running")
					if err != nil {
						cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
					}

				}
			}
		}
		if oldmain != nil {
			err = m.ClearServer(oldmain.MxsServerName, "main")
			if err != nil {
				cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
			}

			if oldmain.State != stateSubordinate {
				err = m.ClearServer(oldmain.MxsServerName, "subordinate")
				if err != nil {
					cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
				}
				err = m.ClearServer(oldmain.MxsServerName, "running")
				if err != nil {
					cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
				}
			} else {
				err = m.SetServer(oldmain.MxsServerName, "subordinate")
				if err != nil {
					cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
				}
				err = m.SetServer(oldmain.MxsServerName, "running")
				if err != nil {
					cluster.LogPrintf(LvlErr, "MaxScale client could not send command:%s", err)
				}

			}
		}
	}
}

func (cluster *Cluster) setMaintenanceMaxscale(pr *Proxy, server *ServerMonitor) {
	m := maxscale.MaxScale{Host: pr.Host, Port: pr.Port, User: pr.User, Pass: pr.Pass}
	err := m.Connect()
	if err != nil {
		cluster.sme.AddState("ERR00018", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00018"], err), ErrFrom: "CONF"})
	}
	if server.IsMaintenance {
		err = m.SetServer(server.MxsServerName, "maintenance")
	} else {
		err = m.ClearServer(server.MxsServerName, "maintenance")
	}
	if err != nil {
		cluster.LogPrintf(LvlErr, "Could not set server %s in maintenance", err)
		m.Close()
	}
	m.Close()
}
