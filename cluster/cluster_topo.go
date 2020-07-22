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
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/signal18/replication-manager/utils/state"
)

type topologyError struct {
	Code int
	Msg  string
}

const (
	topoMainSubordinate         string = "main-subordinate"
	topoUnknown             string = "unknown"
	topoBinlogServer        string = "binlog-server"
	topoMultiTierSubordinate      string = "multi-tier-subordinate"
	topoMultiMain         string = "multi-main"
	topoMultiMainRing     string = "multi-main-ring"
	topoMultiMainWsrep    string = "multi-main-wsrep"
	topoMainSubordinatePgLog    string = "main-subordinate-pg-logical"
	topoMainSubordinatePgStream string = "main-subordinate-pg-stream"
)

func (cluster *Cluster) newServerList() error {
	//sva issue to monitor server should not be fatal

	var err error
	cluster.SetClusterVariablesFromConfig()
	err = cluster.isValidConfig()
	if err != nil {
		cluster.LogPrintf(LvlErr, "Failed to validate config: %s", err)
	}
	//cluster.LogPrintf(LvlErr, "hello %+v", cluster.Conf.Hosts)
	cluster.Lock()
	//cluster.LogPrintf(LvlErr, "hello %+v", cluster.Conf.Hosts)
	cluster.Servers = make([]*ServerMonitor, len(cluster.hostList))
	// split("")  return len = 1
	if cluster.Conf.Hosts != "" {
		slapospartitions := strings.Split(cluster.Conf.SlapOSDBPartitions, ",")
		sstports := strings.Split(cluster.Conf.SchedulerReceiverPorts, ",")

		for k, url := range cluster.hostList {
			cluster.Servers[k], err = cluster.newServerMonitor(url, cluster.dbUser, cluster.dbPass, false, cluster.GetDomain())
			if err != nil {
				cluster.LogPrintf(LvlErr, "Could not open connection to server %s : %s", cluster.Servers[k].URL, err)
			}
			if k < len(slapospartitions) {
				cluster.Servers[k].SlapOSDatadir = slapospartitions[k]
			}

			cluster.Servers[k].SSTPort = sstports[k%len(sstports)]
			if cluster.Conf.Verbose {
				cluster.LogPrintf(LvlInfo, "New database monitored: %v", cluster.Servers[k].URL)
			}
		}
	}
	cluster.Unlock()
	return nil
}

// AddChildServers Add child clusters nodes  if they get same  source name
func (cluster *Cluster) AddChildServers() error {
	mychilds := cluster.GetChildClusters()
	for _, c := range mychilds {
		for _, sv := range c.Servers {

			if sv.IsSubordinateOfReplicationSource(cluster.Conf.MainConn) {
				mymain, _ := cluster.GetMainFromReplication(sv)
				if mymain != nil {
					//	cluster.subordinates = append(cluster.subordinates, sv)
					if !cluster.HasServer(sv) {
						srv, err := cluster.newServerMonitor(sv.Name+":"+sv.Port, sv.ClusterGroup.dbUser, sv.ClusterGroup.dbPass, false, c.GetDomain())
						if err != nil {
							return err
						}
						srv.Ignored = true
						cluster.Servers = append(cluster.Servers, srv)
					}
				}
			}
		}
	}
	return nil
	// End  child clusters  same multi source server discorvery
}

// Start of topology detection
// Create a connection to each host and build list of subordinates.
func (cluster *Cluster) TopologyDiscover(wcg *sync.WaitGroup) error {
	defer wcg.Done()
	cluster.AddChildServers()
	//monitor ignored server fist so that their replication position get oldest
	wg := new(sync.WaitGroup)
	if cluster.Conf.Hosts == "" {
		return errors.New("Can not discover empty cluster")
	}
	for _, server := range cluster.Servers {
		if server.IsIgnored() {
			wg.Add(1)
			go server.Ping(wg)
		}
	}
	wg.Wait()

	wg = new(sync.WaitGroup)
	for _, server := range cluster.Servers {
		if !server.IsIgnored() {
			wg.Add(1)
			go server.Ping(wg)
		}
	}
	wg.Wait()

	//	cluster.pingServerList()
	if cluster.sme.IsInFailover() {
		cluster.LogPrintf(LvlDbg, "In Failover skip topology detection")
		return errors.New("In Failover skip topology detection")
	}
	// Check topology Cluster is down
	cluster.TopologyClusterDown()
	// Check topology Cluster all servers down
	cluster.AllServersFailed()
	cluster.CheckSameServerID()
	// Spider shard discover
	if cluster.Conf.Spider == true {
		cluster.SpiderShardsDiscovery()
	}
	cluster.subordinates = nil
	for k, sv := range cluster.Servers {
		// Failed Do not ignore suspect or topology will change to fast
		if sv.IsFailed() {
			continue
		}
		// count wsrep node as  subordinates
		if sv.IsSubordinate || sv.IsWsrepPrimary {
			if cluster.Conf.LogLevel > 2 {
				cluster.LogPrintf(LvlDbg, "Server %s is configured as a subordinate", sv.URL)
			}
			cluster.subordinates = append(cluster.subordinates, sv)
		} else {
			// not subordinate

			if sv.BinlogDumpThreads == 0 && sv.State != stateMain {
				//sv.State = stateUnconn
				//transition to standalone may happen despite server have never connect successfully when default to suspect
				if cluster.Conf.LogLevel > 2 {
					cluster.LogPrintf(LvlDbg, "Server %s has no subordinates ", sv.URL)
				}
			} else {
				if cluster.Conf.LogLevel > 2 {
					cluster.LogPrintf(LvlDbg, "Server %s was set main as last non subordinate", sv.URL)
				}
				if cluster.Status == ConstMonitorActif && cluster.main != nil && cluster.GetTopology() == topoMainSubordinate && cluster.Servers[k].URL != cluster.main.URL {
					//Extra main in main subordinate topology rejoin it after split brain
					cluster.SetState("ERR00063", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00063"]), ErrFrom: "TOPO"})
					cluster.Servers[k].RejoinMain()
				} else {
					cluster.main = cluster.Servers[k]
					cluster.main.State = stateMain

					if cluster.main.IsReadOnly() && !cluster.main.IsRelay {
						cluster.main.SetReadWrite()
						cluster.LogPrintf(LvlInfo, "Server %s disable read only as last non subordinate", cluster.main.URL)
					}
				}
			}

		}
		// end not subordinate
	}

	// If no cluster.subordinates are detected, generate an error
	if len(cluster.subordinates) == 0 && cluster.GetTopology() != topoMultiMainWsrep {
		cluster.SetState("ERR00010", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00010"]), ErrFrom: "TOPO"})
	}

	// Check that all subordinate servers have the same main and conformity.
	if cluster.Conf.MultiMain == false && cluster.Conf.Spider == false {
		for _, sl := range cluster.subordinates {
			if sl.IsMaxscale == false && !sl.IsFailed() {
				sl.CheckSubordinateSettings()
				sl.CheckSubordinateSameMainGrants()
				if sl.HasCycling() {
					if cluster.Conf.MultiMain == false && len(cluster.Servers) == 2 {
						cluster.SetState("ERR00011", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00011"]), ErrFrom: "TOPO", ServerUrl: sl.URL})
						cluster.Conf.MultiMain = true
					}
					if cluster.Conf.MultiMainRing == false && len(cluster.Servers) > 2 {
						cluster.Conf.MultiMainRing = true
					}
					if cluster.Conf.MultiMainRing == true && cluster.GetMain() == nil {
						cluster.vmain = sl
					}

					//broken replication ring
				} else if cluster.Conf.MultiMainRing == true {
					//setting a virtual main if none
					cluster.SetState("ERR00048", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00048"]), ErrFrom: "TOPO"})
					cluster.main = cluster.GetFailedServer()
				}

			}
			if cluster.Conf.MultiMain == false && sl.IsMaxscale == false {
				if sl.IsSubordinate == true && sl.HasSubordinates(cluster.subordinates) == true {
					sl.IsRelay = true
					sl.State = stateRelay
				} else if sl.IsRelay {
					sl.IsRelay = false
				}
			}
		}
	}
	if cluster.Conf.MultiMain == true || cluster.GetTopology() == topoMultiMainWsrep {
		srw := 0
		for _, s := range cluster.Servers {
			if s.IsReadWrite() {
				srw++
			}
		}
		if srw > 1 {
			cluster.SetState("WARN0003", state.State{ErrType: "WARNING", ErrDesc: "RW server count > 1 in multi-main mode. set read_only=1 in cnf is a must have, choosing prefered main", ErrFrom: "TOPO"})
		}
		srw = 0
		for _, s := range cluster.Servers {
			if s.IsReadOnly() {
				srw++
			}
		}
		if srw > 1 {
			cluster.SetState("WARN0004", state.State{ErrType: "WARNING", ErrDesc: "RO server count > 1 in multi-main mode.  switching to preferred main.", ErrFrom: "TOPO"})
			server := cluster.getPreferedMain()
			if server != nil {
				server.SetReadWrite()
			} else {
				cluster.SetState("WARN0006", state.State{ErrType: "WARNING", ErrDesc: "Multi-main need a preferred main.", ErrFrom: "TOPO"})
			}
		}
	}

	if cluster.subordinates != nil {
		if len(cluster.subordinates) > 0 {
			// Depending if we are doing a failover or a switchover, we will find the main in the list of
			// failed hosts or unconnected hosts.
			// First of all, get a server id from the cluster.subordinates slice, they should be all the same
			sid := cluster.subordinates[0].GetReplicationServerID()

			for k, s := range cluster.Servers {
				if cluster.Conf.MultiMain == false && s.State == stateUnconn {
					if s.ServerID == sid {
						cluster.main = cluster.Servers[k]
						cluster.main.State = stateMain
						cluster.main.SetReadWrite()
						if cluster.Conf.LogLevel > 2 {
							cluster.LogPrintf(LvlDbg, "Server %s was autodetected as a main", s.URL)
						}
						break
					}
				}
				if (cluster.Conf.MultiMain == true || cluster.GetTopology() == topoMultiMainWsrep) && !cluster.Servers[k].IsDown() {
					if s.IsReadWrite() {
						cluster.main = cluster.Servers[k]
						if cluster.Conf.MultiMain == true {
							cluster.main.State = stateMain
						} else {
							cluster.vmain = cluster.Servers[k]
						}
						if cluster.Conf.LogLevel > 2 {
							cluster.LogPrintf(LvlDbg, "Server %s was autodetected as a main", s.URL)
						}
						break
					}
				}
			}

			// If main is not initialized, find it in the failed hosts list
			if cluster.main == nil {
				cluster.FailedMainDiscovery()
			}
		}
	}
	// Final check if main has been found
	if cluster.main == nil {
		// could not detect main
		if cluster.GetMain() == nil {
			cluster.SetState("ERR00012", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00012"]), ErrFrom: "TOPO"})
		}
	} else {
		cluster.main.RplMainStatus = false
		// End of autodetection code
		if !cluster.main.IsDown() {
			cluster.main.CheckMainSettings()
		}
		// Replication checks
		if cluster.Conf.MultiMain == false {
			for _, sl := range cluster.subordinates {

				if sl.IsRelay == false {
					if cluster.Conf.LogLevel > 2 {
						cluster.LogPrintf(LvlDbg, "Checking if server %s is a subordinate of server %s", sl.Host, cluster.main.Host)
					}
					replMain, _ := cluster.GetMainFromReplication(sl)

					if replMain != nil && replMain.Id != cluster.main.Id {
						cluster.SetState("ERR00064", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00064"], sl.URL, cluster.main.URL, replMain.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})

						if cluster.Conf.ReplicationNoRelay && cluster.Status == ConstMonitorActif {
							cluster.RejoinFixRelay(sl, cluster.main)
						}

					}
					if !sl.HasBinlog() {
						cluster.SetState("ERR00013", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00013"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
					}
				}
				if sl.GetReplicationDelay() <= cluster.Conf.FailMaxDelay && sl.IsSQLThreadRunning() {
					cluster.main.RplMainStatus = true
				}

			}
		}
		// State also check in failover_check false positive
		if cluster.main.IsFailed() && cluster.subordinates.HasAllSubordinatesRunning() {
			cluster.SetState("ERR00016", state.State{
				ErrType:   "ERROR",
				ErrDesc:   clusterError["ERR00016"],
				ErrFrom:   "NET",
				ServerUrl: cluster.main.URL,
			})
		}

		cluster.sme.SetMainUpAndSync(cluster.main.SemiSyncMainStatus, cluster.main.RplMainStatus)
	}

	if cluster.HasAllDbUp() {
		if len(cluster.Crashes) > 0 {
			cluster.LogPrintf(LvlDbg, "Purging crashes, all databses nodes up")
			cluster.Crashes = nil
			cluster.Save()
		}
	}
	if cluster.Conf.Arbitration {
		if cluster.IsSplitBrain {
			cluster.SetState("WARN0079", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0079"]), ErrFrom: "ARB"})
		}
		if cluster.IsLostMajority {
			cluster.SetState("WARN0080", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0080"]), ErrFrom: "ARB"})
		}
		if cluster.IsFailedArbitrator {
			cluster.SetState("WARN0090", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0090"], "arbitrator", cluster.Conf.ArbitratorAddress), ErrFrom: "ARB"})
		}
	}
	if cluster.sme.CanMonitor() {
		return nil
	}
	return errors.New("Error found in State Machine Engine")
}

// AllServersDown track state of unvailable cluster
func (cluster *Cluster) AllServersFailed() bool {
	for _, s := range cluster.Servers {
		if s.IsFailed() == false {
			return false
			cluster.IsDown = false
		}
	}
	//"ERR00077": "All databases state down",
	cluster.SetState("ERR00077", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00077"]), ErrFrom: "TOPO"})
	cluster.IsDown = true
	return true
}

// TopologyClusterDown track state of unvailable cluster
func (cluster *Cluster) TopologyClusterDown() bool {
	// search for all cluster down
	if cluster.GetMain() == nil || cluster.GetMain().State == stateFailed {

		allsubordinatefailed := true
		for _, s := range cluster.subordinates {
			if s.State != stateFailed && s.State != stateErrorAuth && !s.IsIgnored() {
				allsubordinatefailed = false
			}
		}
		if allsubordinatefailed {
			if cluster.IsDiscovered() {
				if cluster.main != nil && cluster.Conf.Interactive == false && cluster.Conf.FailRestartUnsafe == false {
					// forget the main if safe mode
					//		cluster.LogPrintf(LvlInfo, "Backing up last seen main: %s for safe failover restart", cluster.main.URL)
					//		cluster.lastmain = cluster.main
					//		cluster.main = nil

				}
			}
			cluster.SetState("ERR00021", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00021"]), ErrFrom: "TOPO"})
			cluster.IsClusterDown = true
			return true
		}
		cluster.IsClusterDown = false

	}
	cluster.IsDown = false
	return false
}

func (cluster *Cluster) PrintTopology() {
	for k, v := range cluster.Servers {
		cluster.LogPrintf(LvlInfo, "Server [%d] %s %s %s", k, v.URL, v.State, v.PrevState)
	}
}

// CountFailed Count number of failed node
func (cluster *Cluster) CountFailed(s []*ServerMonitor) int {
	failed := 0
	for _, server := range cluster.Servers {
		if server.State == stateFailed || server.State == stateErrorAuth {
			failed = failed + 1
		}
	}
	return failed
}

// LostMajority should be call in case of splitbrain to set maintenance mode
func (cluster *Cluster) LostMajority() bool {
	failed := cluster.CountFailed(cluster.Servers)
	alive := len(cluster.Servers) - failed
	if alive > len(cluster.Servers)/2 {
		return false
	} else {
		return true
	}

}

func (cluster *Cluster) FailedMainDiscovery() {

	// Subordinate main_host variable must point to failed main

	smh := cluster.subordinates[0].GetReplicationMainHost()
	for k, s := range cluster.Servers {
		if s.State == stateFailed || s.State == stateErrorAuth {
			if (s.Host == smh || s.IP == smh) && s.Port == cluster.subordinates[0].GetReplicationMainPort() {
				if cluster.Conf.FailRestartUnsafe || cluster.MultipleSubordinatesUp(s) {
					cluster.main = cluster.Servers[k]
					cluster.main.PrevState = stateMain
					cluster.LogPrintf(LvlInfo, "Assuming failed server %s was a main", s.URL)
				}
				break
			}
		}
	}
}

func (cluster *Cluster) MultipleSubordinatesUp(candidate *ServerMonitor) bool {
	ct := 0
	for _, s := range cluster.subordinates {

		if !s.IsDown() && (candidate.Host == s.GetReplicationMainHost() || candidate.IP == s.GetReplicationMainHost()) && candidate.Port == s.GetReplicationMainPort() {
			ct++
		}
	}
	if ct > 0 {
		return true
	}
	return false
}
