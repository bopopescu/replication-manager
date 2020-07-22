// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.

package cluster

import (
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/signal18/replication-manager/config"
	"github.com/signal18/replication-manager/utils/dbhelper"
)

// Bootstrap provisions && setup topology
func (cluster *Cluster) Bootstrap() error {
	var err error
	// create service template and post
	err = cluster.ProvisionServices()
	if err != nil {
		return err
	}
	err = cluster.WaitDatabaseCanConn()
	if err != nil {
		return err
	}

	err = cluster.BootstrapReplication(true)
	if err != nil {
		return err
	}
	if cluster.Conf.Test {
		cluster.initProxies()
		err = cluster.WaitProxyEqualMain()
		if err != nil {
			return err
		}
		err = cluster.WaitBootstrapDiscovery()
		if err != nil {
			return err
		}

		if cluster.GetMain() == nil {
			return errors.New("Abording test, no main found")
		}
		err = cluster.InitBenchTable()
		if err != nil {
			return errors.New("Abording test, can't create bench table")
		}
	}
	return nil
}

func (cluster *Cluster) ProvisionServices() error {

	cluster.sme.SetFailoverState()
	// delete the cluster state here
	path := cluster.WorkingDir + ".json"
	os.Remove(path)
	cluster.ResetCrashes()
	for _, server := range cluster.Servers {
		switch cluster.Conf.ProvOrchestrator {
		case config.ConstOrchestratorOpenSVC:
			go cluster.OpenSVCProvisionDatabaseService(server)
		case config.ConstOrchestratorKubernetes:
			go cluster.K8SProvisionDatabaseService(server)
		case config.ConstOrchestratorSlapOS:
			go cluster.SlapOSProvisionDatabaseService(server)
		case config.ConstOrchestratorLocalhost:
			go cluster.LocalhostProvisionDatabaseService(server)
		default:
			cluster.sme.RemoveFailoverState()
			return nil
		}
	}
	for _, server := range cluster.Servers {
		select {
		case err := <-cluster.errorChan:
			if err != nil {
				cluster.LogPrintf(LvlErr, "Provisionning error %s on  %s", err, cluster.Name+"/svc/"+server.Name)
			} else {
				cluster.LogPrintf(LvlInfo, "Provisionning done for database %s", cluster.Name+"/svc/"+server.Name)
				server.SetProvisionCookie()
				server.DelReprovisionCookie()
				server.DelRestartCookie()
			}
		}
	}
	for _, prx := range cluster.Proxies {
		switch cluster.Conf.ProvOrchestrator {
		case config.ConstOrchestratorOpenSVC:
			go cluster.OpenSVCProvisionProxyService(prx)
		case config.ConstOrchestratorKubernetes:
			go cluster.K8SProvisionProxyService(prx)
		case config.ConstOrchestratorSlapOS:
			go cluster.SlapOSProvisionProxyService(prx)
		case config.ConstOrchestratorLocalhost:
			go cluster.LocalhostProvisionProxyService(prx)
		default:
			cluster.sme.RemoveFailoverState()
			return nil
		}
	}
	for _, prx := range cluster.Proxies {
		select {
		case err := <-cluster.errorChan:
			if err != nil {
				cluster.LogPrintf(LvlErr, "Provisionning proxy error %s on  %s", err, cluster.Name+"/svc/"+prx.Name)
			} else {
				cluster.LogPrintf(LvlInfo, "Provisionning done for proxy %s", cluster.Name+"/svc/"+prx.Name)
				prx.SetProvisionCookie()
			}
		}
	}

	cluster.sme.RemoveFailoverState()

	return nil

}

func (cluster *Cluster) InitDatabaseService(server *ServerMonitor) error {
	cluster.sme.SetFailoverState()
	switch cluster.Conf.ProvOrchestrator {
	case config.ConstOrchestratorOpenSVC:
		go cluster.OpenSVCProvisionDatabaseService(server)
	case config.ConstOrchestratorKubernetes:
		go cluster.K8SProvisionDatabaseService(server)
	case config.ConstOrchestratorSlapOS:
		go cluster.SlapOSProvisionDatabaseService(server)
	case config.ConstOrchestratorLocalhost:
		go cluster.LocalhostProvisionDatabaseService(server)
	default:
		cluster.sme.RemoveFailoverState()
		return nil
	}
	select {
	case err := <-cluster.errorChan:
		cluster.sme.RemoveFailoverState()
		if err == nil {
			server.SetProvisionCookie()
		} else {
		}
		return err
	}

	return nil
}

func (cluster *Cluster) InitProxyService(prx *Proxy) error {
	switch cluster.Conf.ProvOrchestrator {
	case config.ConstOrchestratorOpenSVC:
		go cluster.OpenSVCProvisionProxyService(prx)
	case config.ConstOrchestratorKubernetes:
		go cluster.K8SProvisionProxyService(prx)
	case config.ConstOrchestratorSlapOS:
		go cluster.SlapOSProvisionProxyService(prx)
	case config.ConstOrchestratorLocalhost:
		go cluster.LocalhostProvisionProxyService(prx)
	default:
		return nil
	}
	select {
	case err := <-cluster.errorChan:
		cluster.sme.RemoveFailoverState()
		if err == nil {
			prx.SetProvisionCookie()
		}
		return err
	}
	return nil
}

func (cluster *Cluster) Unprovision() error {

	cluster.sme.SetFailoverState()
	for _, server := range cluster.Servers {
		switch cluster.Conf.ProvOrchestrator {
		case config.ConstOrchestratorOpenSVC:
			go cluster.OpenSVCUnprovisionDatabaseService(server)
		case config.ConstOrchestratorKubernetes:
			go cluster.K8SUnprovisionDatabaseService(server)
		case config.ConstOrchestratorSlapOS:
			go cluster.SlapOSUnprovisionDatabaseService(server)
		case config.ConstOrchestratorLocalhost:
			go cluster.LocalhostUnprovisionDatabaseService(server)
		default:
			cluster.sme.RemoveFailoverState()
			return nil
		}
	}
	for _, server := range cluster.Servers {
		select {
		case err := <-cluster.errorChan:
			if err != nil {
				cluster.LogPrintf(LvlErr, "Unprovision error %s on  %s", err, cluster.Name+"/svc/"+server.Name)
			} else {
				cluster.LogPrintf(LvlInfo, "Unprovision done for database %s", cluster.Name+"/svc/"+server.Name)
				server.DelProvisionCookie()
				server.DelRestartCookie()
				server.DelReprovisionCookie()
			}
		}
	}
	for _, prx := range cluster.Proxies {
		switch cluster.Conf.ProvOrchestrator {
		case config.ConstOrchestratorOpenSVC:
			go cluster.OpenSVCUnprovisionProxyService(prx)
		case config.ConstOrchestratorKubernetes:
			go cluster.K8SUnprovisionProxyService(prx)
		case config.ConstOrchestratorSlapOS:
			go cluster.SlapOSUnprovisionProxyService(prx)
		case config.ConstOrchestratorLocalhost:
			go cluster.LocalhostUnprovisionProxyService(prx)
		default:
			cluster.sme.RemoveFailoverState()
			return nil
		}
	}
	for _, prx := range cluster.Proxies {
		select {
		case err := <-cluster.errorChan:
			if err != nil {
				cluster.LogPrintf(LvlErr, "Unprovision proxy error %s on  %s", err, cluster.Name+"/svc/"+prx.Name)
			} else {
				cluster.LogPrintf(LvlInfo, "Unprovision done for proxy %s", cluster.Name+"/svc/"+prx.Name)
				prx.DelProvisionCookie()
				prx.DelRestartCookie()
				prx.DelReprovisionCookie()
			}
		}
	}

	cluster.subordinates = nil
	cluster.main = nil
	cluster.vmain = nil
	cluster.IsAllDbUp = false
	cluster.sme.UnDiscovered()
	cluster.sme.RemoveFailoverState()
	return nil
}

func (cluster *Cluster) UnprovisionProxyService(prx *Proxy) error {
	switch cluster.Conf.ProvOrchestrator {
	case config.ConstOrchestratorOpenSVC:
		go cluster.OpenSVCUnprovisionProxyService(prx)
	case config.ConstOrchestratorKubernetes:
		go cluster.K8SUnprovisionProxyService(prx)
	case config.ConstOrchestratorSlapOS:
		go cluster.SlapOSUnprovisionProxyService(prx)
	case config.ConstOrchestratorLocalhost:
		go cluster.LocalhostUnprovisionProxyService(prx)
	default:
	}
	select {
	case err := <-cluster.errorChan:
		if err == nil {
			prx.DelProvisionCookie()
			prx.DelReprovisionCookie()
			prx.DelRestartCookie()
		}
		return err
	}
	return nil
}

func (cluster *Cluster) UnprovisionDatabaseService(server *ServerMonitor) error {
	cluster.ResetCrashes()
	switch cluster.Conf.ProvOrchestrator {
	case config.ConstOrchestratorOpenSVC:
		go cluster.OpenSVCUnprovisionDatabaseService(server)
	case config.ConstOrchestratorKubernetes:
		go cluster.K8SUnprovisionDatabaseService(server)
	case config.ConstOrchestratorSlapOS:
		go cluster.SlapOSUnprovisionDatabaseService(server)
	default:
		go cluster.LocalhostUnprovisionDatabaseService(server)
	}
	select {

	case err := <-cluster.errorChan:
		if err == nil {
			server.DelProvisionCookie()
			server.DelReprovisionCookie()
			server.DelRestartCookie()
		}
		return err
	}
	return nil
}

func (cluster *Cluster) RollingUpgrade() {
}

func (cluster *Cluster) StopDatabaseService(server *ServerMonitor) error {

	switch cluster.Conf.ProvOrchestrator {
	case config.ConstOrchestratorOpenSVC:
		return cluster.OpenSVCStopDatabaseService(server)
	case config.ConstOrchestratorKubernetes:
		cluster.K8SStopDatabaseService(server)
	case config.ConstOrchestratorSlapOS:
		cluster.SlapOSStopDatabaseService(server)
	case config.ConstOrchestratorOnPremise:
		cluster.OnPremiseStopDatabaseService(server)
	case config.ConstOrchestratorLocalhost:
		return cluster.LocalhostStopDatabaseService(server)
	default:
		return errors.New("No valid orchestrator")
	}
	server.DelRestartCookie()
	return nil
}

func (cluster *Cluster) StopProxyService(server *Proxy) error {

	switch cluster.Conf.ProvOrchestrator {
	case config.ConstOrchestratorOpenSVC:
		return cluster.OpenSVCStopProxyService(server)
	case config.ConstOrchestratorKubernetes:
		cluster.K8SStopProxyService(server)
	case config.ConstOrchestratorSlapOS:
		cluster.SlapOSStopProxyService(server)
	default:
		return cluster.LocalhostStopProxyService(server)
	}
	server.DelRestartCookie()
	return nil
}

func (cluster *Cluster) StartProxyService(server *Proxy) error {

	switch cluster.Conf.ProvOrchestrator {
	case config.ConstOrchestratorOpenSVC:
		return cluster.OpenSVCStartProxyService(server)
	case config.ConstOrchestratorKubernetes:
		cluster.K8SStartProxyService(server)
	case config.ConstOrchestratorSlapOS:
		cluster.SlapOSStartProxyService(server)
	default:
		return cluster.LocalhostStartProxyService(server)
	}
	server.DelRestartCookie()
	return nil
}

func (cluster *Cluster) ShutdownDatabase(server *ServerMonitor) error {
	_, err := server.Conn.Exec("SHUTDOWN")
	server.DelRestartCookie()
	return err
}

func (cluster *Cluster) StartDatabaseService(server *ServerMonitor) error {
	cluster.LogPrintf(LvlInfo, "Starting Database service %s", cluster.Name+"/svc/"+server.Name)
	switch cluster.Conf.ProvOrchestrator {
	case config.ConstOrchestratorOpenSVC:
		return cluster.OpenSVCStartDatabaseService(server)
	case config.ConstOrchestratorKubernetes:
		cluster.K8SStartDatabaseService(server)
	case config.ConstOrchestratorSlapOS:
		cluster.SlapOSStartDatabaseService(server)
	case config.ConstOrchestratorOnPremise:
		cluster.OnPremiseStartDatabaseService(server)
	case config.ConstOrchestratorLocalhost:
		return cluster.LocalhostStartDatabaseService(server)
	default:
		return errors.New("No valid orchestrator")
	}
	server.DelRestartCookie()
	return nil
}

func (cluster *Cluster) GetOchestaratorPlacement(server *ServerMonitor) error {
	cluster.LogPrintf(LvlInfo, "Starting Database service %s", cluster.Name+"/svc/"+server.Name)
	switch cluster.Conf.ProvOrchestrator {
	case config.ConstOrchestratorOpenSVC:
		return cluster.OpenSVCStartDatabaseService(server)
	case config.ConstOrchestratorKubernetes:
		cluster.K8SStartDatabaseService(server)
	case config.ConstOrchestratorSlapOS:
		cluster.SlapOSStartDatabaseService(server)
	case config.ConstOrchestratorLocalhost:
		return cluster.LocalhostStartDatabaseService(server)
	default:
		return errors.New("No valid orchestrator")
	}
	return nil
}

func (cluster *Cluster) StartAllNodes() error {

	return nil
}

func (cluster *Cluster) BootstrapReplicationCleanup() error {

	cluster.LogPrintf(LvlInfo, "Cleaning up replication on existing servers")
	cluster.sme.SetFailoverState()
	for _, server := range cluster.Servers {
		err := server.Refresh()
		if err != nil {
			cluster.LogPrintf(LvlErr, "Refresh failed in Cleanup on server %s %s", server.URL, err)
			return err
		}
		if cluster.Conf.Verbose {
			cluster.LogPrintf(LvlInfo, "SetDefaultMainConn on server %s ", server.URL)
		}
		logs, err := dbhelper.SetDefaultMainConn(server.Conn, cluster.Conf.MainConn, server.DBVersion)
		cluster.LogSQL(logs, err, server.URL, "BootstrapReplicationCleanup", LvlDbg, "BootstrapReplicationCleanup %s %s ", server.URL, err)
		if err != nil {
			if cluster.Conf.Verbose {
				cluster.LogPrintf(LvlInfo, "RemoveFailoverState on server %s ", server.URL)
			}
			continue
		}

		cluster.LogPrintf(LvlInfo, "Reset Main on server %s ", server.URL)

		logs, err = dbhelper.ResetMain(server.Conn, cluster.Conf.MainConn, server.DBVersion)
		cluster.LogSQL(logs, err, server.URL, "BootstrapReplicationCleanup", LvlErr, "Reset Main on server %s %s", server.URL, err)
		if cluster.Conf.Verbose {
			cluster.LogPrintf(LvlInfo, "Stop all subordinates or stop subordinate %s ", server.URL)
		}
		if server.DBVersion.IsMariaDB() {
			logs, err = dbhelper.StopAllSubordinates(server.Conn, server.DBVersion)
		} else {
			logs, err = server.StopSubordinate()
		}
		cluster.LogSQL(logs, err, server.URL, "BootstrapReplicationCleanup", LvlErr, "Stop all subordinates or just subordinate %s %s", server.URL, err)

		if server.DBVersion.IsMariaDB() {
			if cluster.Conf.Verbose {
				cluster.LogPrintf(LvlInfo, "SET GLOBAL gtid_subordinate_pos='' on %s", server.URL)
			}
			logs, err := dbhelper.SetGTIDSubordinatePos(server.Conn, "")
			cluster.LogSQL(logs, err, server.URL, "BootstrapReplicationCleanup", LvlErr, "Can reset GTID subordinate pos %s %s", server.URL, err)
		}

	}
	cluster.main = nil
	cluster.vmain = nil
	cluster.subordinates = nil
	cluster.sme.RemoveFailoverState()
	return nil
}

func (cluster *Cluster) BootstrapReplication(clean bool) error {

	// default to main subordinate
	var err error

	if cluster.Conf.MultiMainWsrep {
		cluster.LogPrintf(LvlInfo, "Galera cluster ignoring replication setup")
		return nil
	}
	if clean {
		err := cluster.BootstrapReplicationCleanup()
		if err != nil {
			cluster.LogPrintf(LvlErr, "Cleanup error %s", err)
		}
	}
	for _, server := range cluster.Servers {
		if server.State == stateFailed {
			continue
		} else {
			server.Refresh()
		}
	}
	wg := new(sync.WaitGroup)
	wg.Add(1)
	err = cluster.TopologyDiscover(wg)
	wg.Wait()
	if err == nil {
		return errors.New("Environment already has an existing main/subordinate setup")
	}

	cluster.sme.SetFailoverState()
	mainKey := 0
	if cluster.Conf.PrefMain != "" {
		mainKey = func() int {
			for k, server := range cluster.Servers {
				if server.IsPrefered() {
					cluster.sme.RemoveFailoverState()
					return k
				}
			}
			cluster.sme.RemoveFailoverState()
			return -1
		}()
	}
	if mainKey == -1 {
		return errors.New("Preferred main could not be found in existing servers")
	}

	// Assume main-subordinate if nothing else is declared
	if cluster.Conf.MultiMainRing == false && cluster.Conf.MultiMain == false && cluster.Conf.MxsBinlogOn == false && cluster.Conf.MultiTierSubordinate == false {

		for key, server := range cluster.Servers {
			if server.State == stateFailed {
				continue
			}
			if key == mainKey {
				dbhelper.FlushTables(server.Conn)
				server.SetReadWrite()
				continue
			} else {
				err = server.ChangeMainTo(cluster.Servers[mainKey], "SLAVE_POS")
				if !server.ClusterGroup.IsInIgnoredReadonly(server) {
					server.SetReadOnly()
				}
			}

		}
	}
	// Subordinate Relay
	if cluster.Conf.MultiTierSubordinate == true {
		mainKey = 0
		relaykey := 1
		for key, server := range cluster.Servers {
			if server.State == stateFailed {
				continue
			}
			if key == mainKey {
				dbhelper.FlushTables(server.Conn)
				server.SetReadWrite()
				continue
			} else {
				dbhelper.StopAllSubordinates(server.Conn, server.DBVersion)
				dbhelper.ResetAllSubordinates(server.Conn, server.DBVersion)

				if relaykey == key {
					err = server.ChangeMainTo(cluster.Servers[mainKey], "CURRENT_POS")
					if err != nil {
						cluster.sme.RemoveFailoverState()
						return err
					}

				} else {
					err = server.ChangeMainTo(cluster.Servers[relaykey], "CURRENT_POS")
					if err != nil {
						cluster.sme.RemoveFailoverState()
						return err
					}
				}
				if !server.ClusterGroup.IsInIgnoredReadonly(server) {
					server.SetReadOnly()
				}
			}
		}
		cluster.LogPrintf(LvlInfo, "Environment bootstrapped with %s as main", cluster.Servers[mainKey].URL)
	}
	// Multi Main
	if cluster.Conf.MultiMain == true {
		for key, server := range cluster.Servers {
			if server.State == stateFailed {
				continue
			}
			if key == 0 {
				err = server.ChangeMainTo(cluster.Servers[1], "CURRENT_POS")
				if err != nil {
					cluster.sme.RemoveFailoverState()
					return err
				}
				if !server.ClusterGroup.IsInIgnoredReadonly(server) {
					server.SetReadOnly()
				}
			}
			if key == 1 {
				err = server.ChangeMainTo(cluster.Servers[0], "CURRENT_POS")
				if err != nil {
					cluster.sme.RemoveFailoverState()
					return err
				}
			}
			if !server.ClusterGroup.IsInIgnoredReadonly(server) {
				server.SetReadOnly()
			}
		}
	}
	// Ring
	if cluster.Conf.MultiMainRing == true {
		for key, server := range cluster.Servers {
			if server.State == stateFailed {
				continue
			}
			i := (len(cluster.Servers) + key - 1) % len(cluster.Servers)
			err = server.ChangeMainTo(cluster.Servers[i], "SLAVE_POS")
			if err != nil {
				cluster.sme.RemoveFailoverState()
				return err
			}

			cluster.vmain = cluster.Servers[0]

		}
	}
	cluster.sme.RemoveFailoverState()
	// speed up topology discovery
	wg.Add(1)
	cluster.TopologyDiscover(wg)
	wg.Wait()

	//bootstrapChan <- true
	return nil
}

func (cluster *Cluster) GetDatabaseAgent(server *ServerMonitor) (Agent, error) {
	var agent Agent
	agents := strings.Split(cluster.Conf.ProvAgents, ",")
	if len(agents) == 0 {
		return agent, errors.New("No databases agent list provided")
	}
	for i, srv := range cluster.Servers {

		if srv.Id == server.Id {
			agentName := agents[i%len(agents)]
			agent, err := cluster.GetAgentInOrchetrator(agentName)
			if err != nil {
				return agent, err
			} else {
				return agent, nil
			}
		}
	}
	return agent, errors.New("Indice not found in database node list")
}

func (cluster *Cluster) GetProxyAgent(server *Proxy) (Agent, error) {
	var agent Agent
	agents := strings.Split(cluster.Conf.ProvProxAgents, ",")
	if len(agents) == 0 {
		return agent, errors.New("No databases agent list provided")
	}
	for i, srv := range cluster.Servers {

		if srv.Id == server.Id {
			agentName := agents[i%len(agents)]
			agent, err := cluster.GetAgentInOrchetrator(agentName)
			if err != nil {
				return agent, err
			} else {
				return agent, nil
			}
		}
	}
	return agent, errors.New("Indice not found in database node list")
}

func (cluster *Cluster) GetAgentInOrchetrator(name string) (Agent, error) {
	var node Agent
	for _, node := range cluster.Agents {
		if name == node.HostName {
			return node, nil
		}
	}
	return node, errors.New("Agent not found in orechestrator node list")
}
