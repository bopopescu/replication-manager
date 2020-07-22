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
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/signal18/replication-manager/utils/dbhelper"
	"github.com/signal18/replication-manager/utils/gtid"
	"github.com/signal18/replication-manager/utils/state"
)

// MainFailover triggers a main switchover and returns the new main URL
func (cluster *Cluster) MainFailover(fail bool) bool {
	if cluster.GetTopology() == topoMultiMainRing || cluster.GetTopology() == topoMultiMainWsrep {
		res := cluster.VMainFailover(fail)
		return res
	}
	cluster.sme.SetFailoverState()
	// Phase 1: Cleanup and election
	var err error
	if fail == false {
		cluster.LogPrintf(LvlInfo, "--------------------------")
		cluster.LogPrintf(LvlInfo, "Starting main switchover")
		cluster.LogPrintf(LvlInfo, "--------------------------")
		cluster.LogPrintf(LvlInfo, "Checking long running updates on main %d", cluster.Conf.SwitchWaitWrite)
		if cluster.main == nil {
			cluster.LogPrintf(LvlErr, "Cannot switchover without a main")
			return false
		}
		if cluster.main.Conn == nil {
			cluster.LogPrintf(LvlErr, "Cannot switchover without a main connection")
			return false
		}
		qt, logs, err := dbhelper.CheckLongRunningWrites(cluster.main.Conn, cluster.Conf.SwitchWaitWrite)
		cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlDbg, "CheckLongRunningWrites")
		if qt > 0 {
			cluster.LogPrintf(LvlErr, "Long updates running on main. Cannot switchover")
			cluster.sme.RemoveFailoverState()
			return false
		}

		cluster.LogPrintf(LvlInfo, "Flushing tables on main %s", cluster.main.URL)
		workerFlushTable := make(chan error, 1)
		if cluster.main.DBVersion.IsMariaDB() && cluster.main.DBVersion.Major > 10 && cluster.main.DBVersion.Minor >= 1 {

			go func() {
				var err2 error
				logs, err2 = dbhelper.MariaDBFlushTablesNoLogTimeout(cluster.main.Conn, strconv.FormatInt(cluster.Conf.SwitchWaitTrx+2, 10))
				cluster.LogSQL(logs, err2, cluster.main.URL, "MainFailover", LvlDbg, "MariaDBFlushTablesNoLogTimeout")
				workerFlushTable <- err2
			}()
		} else {
			go func() {
				var err2 error
				logs, err2 = dbhelper.FlushTablesNoLog(cluster.main.Conn)
				cluster.LogSQL(logs, err2, cluster.main.URL, "MainFailover", LvlDbg, "FlushTablesNoLog")
				workerFlushTable <- err2
			}()

		}

		select {
		case err = <-workerFlushTable:
			if err != nil {
				cluster.LogPrintf(LvlWarn, "Could not flush tables on main", err)
			}
		case <-time.After(time.Second * time.Duration(cluster.Conf.SwitchWaitTrx)):
			cluster.LogPrintf(LvlErr, "Long running trx on main at least %d, can not switchover ", cluster.Conf.SwitchWaitTrx)
			cluster.sme.RemoveFailoverState()
			return false
		}

	} else {
		cluster.LogPrintf(LvlInfo, "------------------------")
		cluster.LogPrintf(LvlInfo, "Starting main failover")
		cluster.LogPrintf(LvlInfo, "------------------------")
	}
	cluster.LogPrintf(LvlInfo, "Electing a new main")
	for _, s := range cluster.subordinates {
		s.Refresh()
	}
	key := -1
	if fail {
		key = cluster.electFailoverCandidate(cluster.subordinates, true)
	} else {
		key = cluster.electSwitchoverCandidate(cluster.subordinates, true)
	}
	if key == -1 {
		cluster.LogPrintf(LvlErr, "No candidates found")
		cluster.sme.RemoveFailoverState()
		return false
	}

	cluster.LogPrintf(LvlInfo, "Subordinate %s has been elected as a new main", cluster.subordinates[key].URL)
	if fail && !cluster.isSubordinateElectable(cluster.subordinates[key], true) {
		cluster.LogPrintf(LvlInfo, "Elected subordinate have issue cancelling failover", cluster.subordinates[key].URL)
		cluster.sme.RemoveFailoverState()
		return false
	}
	// Shuffle the server list
	var skey int
	for k, server := range cluster.Servers {
		if cluster.subordinates[key].URL == server.URL {
			skey = k
			break
		}
	}
	cluster.oldMain = cluster.main
	cluster.main = cluster.Servers[skey]
	cluster.main.State = stateMain
	if cluster.Conf.MultiMain == false {
		cluster.subordinates[key].delete(&cluster.subordinates)
	}
	// Call pre-failover script
	if cluster.Conf.PreScript != "" {
		cluster.LogPrintf(LvlInfo, "Calling pre-failover script")
		var out []byte
		out, err = exec.Command(cluster.Conf.PreScript, cluster.oldMain.Host, cluster.main.Host, cluster.oldMain.Port, cluster.main.Port, cluster.oldMain.MxsServerName, cluster.main.MxsServerName).CombinedOutput()
		if err != nil {
			cluster.LogPrintf(LvlErr, "%s", err)
		}
		cluster.LogPrintf(LvlInfo, "Pre-failover script complete:", string(out))
	}

	// Phase 2: Reject updates and sync subordinates on switchover
	if fail == false {
		if cluster.Conf.FailEventStatus {
			for _, v := range cluster.main.EventStatus {
				if v.Status == 3 {
					cluster.LogPrintf(LvlInfo, "Set DISABLE ON SLAVE for event %s %s on old main", v.Db, v.Name)
					logs, err := dbhelper.SetEventStatus(cluster.oldMain.Conn, v, 3)
					cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not Set DISABLE ON SLAVE for event %s %s on old main", v.Db, v.Name)
				}
			}
		}
		if cluster.Conf.FailEventScheduler {
			cluster.LogPrintf(LvlInfo, "Disable Event Scheduler on old main")
			logs, err := dbhelper.SetEventScheduler(cluster.oldMain.Conn, false, cluster.oldMain.DBVersion)
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not disable event scheduler on old main")
		}
		cluster.oldMain.freeze()
		cluster.LogPrintf(LvlInfo, "Rejecting updates on %s (old main)", cluster.oldMain.URL)
		logs, err := dbhelper.FlushTablesWithReadLock(cluster.oldMain.Conn, cluster.oldMain.DBVersion)
		cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not lock tables on %s (old main) %s", cluster.oldMain.URL, err)

	}
	// Sync candidate depending on the main status.
	// If it's a switchover, use MASTER_POS_WAIT to sync.
	// If it's a failover, wait for the SQL thread to read all relay logs.
	// If maxsclale we should wait for relay catch via old style
	crash := new(Crash)
	crash.URL = cluster.oldMain.URL
	crash.ElectedMainURL = cluster.main.URL

	// if switchover on MariaDB Wait GTID
	/*	if fail == false && cluster.Conf.MxsBinlogOn == false && cluster.main.DBVersion.IsMariaDB() {
		cluster.LogPrintf(LvlInfo, "Waiting for candidate Main to synchronize")
		cluster.oldMain.Refresh()
		if cluster.Conf.LogLevel > 2 {
			cluster.LogPrintf(LvlDbg, "Syncing on main GTID Binlog Pos [%s]", cluster.oldMain.GTIDBinlogPos.Sprint())
			cluster.oldMain.log()
		}
		dbhelper.MainWaitGTID(cluster.main.Conn, cluster.oldMain.GTIDBinlogPos.Sprint(), 30)
	} else {*/
	// Failover
	cluster.LogPrintf(LvlInfo, "Waiting for candidate main to apply relay log")
	err = cluster.main.ReadAllRelayLogs()
	if err != nil {
		cluster.LogPrintf(LvlErr, "Error while reading relay logs on candidate: %s", err)
	}
	cluster.LogPrintf(LvlDbg, "Save replication status before electing")
	ms, err := cluster.main.GetSubordinateStatus(cluster.main.ReplicationSourceName)
	if err != nil {
		cluster.LogPrintf(LvlErr, "Failover can not fetch replication info on new main: %s", err)
	}
	cluster.LogPrintf(LvlDbg, "main_log_file=%s", ms.MainLogFile.String)
	cluster.LogPrintf(LvlDbg, "main_log_pos=%s", ms.ReadMainLogPos.String)
	cluster.LogPrintf(LvlDbg, "Candidate was in sync=%t", cluster.main.SemiSyncSubordinateStatus)
	//		cluster.main.FailoverMainLogFile = cluster.main.MainLogFile
	//		cluster.main.FailoverMainLogPos = cluster.main.MainLogPos
	crash.FailoverMainLogFile = ms.MainLogFile.String
	crash.FailoverMainLogPos = ms.ReadMainLogPos.String
	crash.NewMainLogFile = cluster.main.BinaryLogFile
	crash.NewMainLogPos = cluster.main.BinaryLogPos
	if cluster.main.DBVersion.IsMariaDB() {
		if cluster.Conf.MxsBinlogOn {
			//	cluster.main.FailoverIOGtid = cluster.main.CurrentGtid
			crash.FailoverIOGtid = cluster.main.CurrentGtid
		} else {
			//	cluster.main.FailoverIOGtid = gtid.NewList(ms.GtidIOPos.String)
			crash.FailoverIOGtid = gtid.NewList(ms.GtidIOPos.String)
		}
	} else if cluster.main.DBVersion.IsMySQLOrPerconaGreater57() && cluster.main.HasGTIDReplication() {
		crash.FailoverIOGtid = gtid.NewMySQLList(ms.ExecutedGtidSet.String)
	}
	cluster.main.FailoverSemiSyncSubordinateStatus = cluster.main.SemiSyncSubordinateStatus
	crash.FailoverSemiSyncSubordinateStatus = cluster.main.SemiSyncSubordinateStatus
	//}

	// if relay server than failover and switchover converge to a new binlog  make this happen
	var relaymain *ServerMonitor
	if cluster.Conf.MxsBinlogOn || cluster.Conf.MultiTierSubordinate {
		cluster.LogPrintf(LvlInfo, "Candidate main has to catch up with relay server log position")
		relaymain = cluster.GetRelayServer()
		if relaymain != nil {
			rs, err := relaymain.GetSubordinateStatus(relaymain.ReplicationSourceName)
			if err != nil {
				cluster.LogPrintf(LvlErr, "Can't find subordinate status on relay server %s", relaymain.URL)
			}
			relaymain.Refresh()

			binlogfiletoreach, _ := strconv.Atoi(strings.Split(rs.MainLogFile.String, ".")[1])
			cluster.LogPrintf(LvlInfo, "Relay server log pos reached %d", binlogfiletoreach)
			logs, err := dbhelper.ResetMain(cluster.main.Conn, cluster.Conf.MainConn, cluster.main.DBVersion)
			cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlInfo, "Reset Main on candidate Main")
			ctbinlog := 0
			for ctbinlog < binlogfiletoreach {
				ctbinlog++
				logs, err := dbhelper.FlushLogs(cluster.main.Conn)
				cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlInfo, "Flush Log on new Main %d", ctbinlog)
			}
			time.Sleep(2 * time.Second)
			ms, logs, err := dbhelper.GetMainStatus(cluster.main.Conn, cluster.main.DBVersion)
			cluster.main.FailoverMainLogFile = ms.File
			cluster.main.FailoverMainLogPos = "4"
			crash.FailoverMainLogFile = ms.File
			crash.FailoverMainLogPos = "4"
			cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlInfo, "Backing up main pos %s %s", crash.FailoverMainLogFile, crash.FailoverMainLogPos)

		} else {
			cluster.LogPrintf(LvlErr, "No relay server found")
		}
	}
	// Phase 3: Prepare new main
	if cluster.Conf.MultiMain == false {
		cluster.LogPrintf(LvlInfo, "Stopping subordinate threads on new main")
		if cluster.main.DBVersion.IsMariaDB() || (cluster.main.DBVersion.IsMariaDB() == false && cluster.main.DBVersion.Minor < 7) {
			logs, err := cluster.main.StopSubordinate()
			cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlErr, "Failed stopping subordinate on new main %s %s", cluster.main.URL, err)
		}
	}
	cluster.Crashes = append(cluster.Crashes, crash)
	t := time.Now()
	crash.Save(cluster.WorkingDir + "/failover." + t.Format("20060102150405") + ".json")
	crash.Purge(cluster.WorkingDir, cluster.Conf.FailoverLogFileKeep)
	cluster.Save()
	// Call post-failover script before unlocking the old main.
	if cluster.Conf.PostScript != "" {
		cluster.LogPrintf(LvlInfo, "Calling post-failover script")
		var out []byte
		out, err = exec.Command(cluster.Conf.PostScript, cluster.oldMain.Host, cluster.main.Host, cluster.oldMain.Port, cluster.main.Port, cluster.oldMain.MxsServerName, cluster.main.MxsServerName).CombinedOutput()
		if err != nil {
			cluster.LogPrintf(LvlErr, "%s", err)
		}
		cluster.LogPrintf(LvlInfo, "Post-failover script complete", string(out))
	}

	if cluster.Conf.MultiMain == false {
		cluster.LogPrintf(LvlInfo, "Resetting subordinate on new main and set read/write mode on")
		if cluster.main.DBVersion.IsMySQLOrPercona() {
			// Need to stop all threads to reset on MySQL
			logs, err := cluster.main.StopSubordinate()
			cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlErr, "Failed stop subordinate on new main %s %s", cluster.main.URL, err)
		}

		logs, err := cluster.main.ResetSubordinate()
		cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlErr, "Failed reset subordinate on new main %s %s", cluster.main.URL, err)
	}
	if fail == false {
		// Get Fresh GTID pos before open traffic
		cluster.main.Refresh()
	}
	err = cluster.main.SetReadWrite()
	if err != nil {
		cluster.LogPrintf(LvlErr, "Could not set new main as read-write")
	}
	cluster.LogPrintf(LvlInfo, "Failover proxies")
	cluster.failoverProxies()
	cluster.LogPrintf(LvlInfo, "Waiting %ds for unmanaged proxy to monitor route change", cluster.Conf.SwitchSubordinateWaitRouteChange)
	time.Sleep(time.Duration(cluster.Conf.SwitchSubordinateWaitRouteChange) * time.Second)
	if cluster.Conf.FailEventScheduler {
		cluster.LogPrintf(LvlInfo, "Enable Event Scheduler on the new main")
		logs, err := dbhelper.SetEventScheduler(cluster.main.Conn, true, cluster.main.DBVersion)
		cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlErr, "Could not enable event scheduler on the new main")
	}
	if cluster.Conf.FailEventStatus {
		for _, v := range cluster.main.EventStatus {
			if v.Status == 3 {
				cluster.LogPrintf(LvlInfo, "Set ENABLE for event %s %s on new main", v.Db, v.Name)
				logs, err := dbhelper.SetEventStatus(cluster.main.Conn, v, 1)
				cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlErr, "Could not Set ENABLE for event %s %s on new main", v.Db, v.Name)
			}
		}
	}
	// Insert a bogus transaction in order to have a new GTID pos on main
	cluster.LogPrintf(LvlInfo, "Inject fake transaction on new main %s ", cluster.main.URL)
	logs, err := dbhelper.FlushTables(cluster.main.Conn)
	cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlErr, "Could not flush tables on new main for fake trx %s", err)

	if fail == false {
		// Get latest GTID pos
		//cluster.main.Refresh() moved just before opening writes
		cluster.oldMain.Refresh()

		// ********
		// Phase 4: Demote old main to subordinate
		// ********
		cluster.LogPrintf(LvlInfo, "Killing new connections on old main showing before update route")
		dbhelper.KillThreads(cluster.oldMain.Conn, cluster.oldMain.DBVersion)
		cluster.LogPrintf(LvlInfo, "Switching old main as a subordinate")
		logs, err := dbhelper.UnlockTables(cluster.oldMain.Conn)
		cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not unlock tables on old main %s", err)

		cluster.oldMain.StopSubordinate() // This is helpful in some cases the old main can have an old replication running
		one_shoot_subordinate_pos := false
		if cluster.oldMain.DBVersion.IsMariaDB() && cluster.oldMain.HaveMariaDBGTID == false && cluster.oldMain.DBVersion.Major >= 10 {
			logs, err := dbhelper.SetGTIDSubordinatePos(cluster.oldMain.Conn, cluster.main.GTIDBinlogPos.Sprint())
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not set old main gtid_subordinate_pos , reason: %s", err)
			one_shoot_subordinate_pos = true
		}
		hasMyGTID := cluster.oldMain.HasMySQLGTID()
		cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not check old main GTID status: %s", err)
		var changeMainErr error
		// Do positional switch if we are not MariaDB and no using GTID
		if cluster.oldMain.DBVersion.IsMariaDB() == false && hasMyGTID == false {
			cluster.LogPrintf(LvlInfo, "Doing positional switch of old Main")
			logs, changeMainErr = dbhelper.ChangeMain(cluster.oldMain.Conn, dbhelper.ChangeMainOpt{
				Host:        cluster.main.Host,
				Port:        cluster.main.Port,
				User:        cluster.rplUser,
				Password:    cluster.rplPass,
				Logfile:     cluster.main.BinaryLogFile,
				Logpos:      cluster.main.BinaryLogPos,
				Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
				Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
				Mode:        "POSITIONAL",
				SSL:         cluster.Conf.ReplicationSSL,
				Channel:     cluster.Conf.MainConn,
				IsDelayed:   cluster.oldMain.IsDelayed,
				Delay:       strconv.Itoa(cluster.oldMain.ClusterGroup.Conf.HostsDelayedTime),
				PostgressDB: cluster.main.PostgressDB,
			}, cluster.oldMain.DBVersion)
			cluster.LogSQL(logs, changeMainErr, cluster.oldMain.URL, "MainFailover", LvlErr, "Change main failed on old main, reason:%s ", changeMainErr)

			logs, err = cluster.oldMain.StartSubordinate()
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Start subordinate failed on old main,%s reason:  %s ", cluster.oldMain.URL, err)

		} else if hasMyGTID == true {
			// We can do MySQL 5.7 style failover
			cluster.LogPrintf(LvlInfo, "Doing MySQL GTID switch of the old main")
			logs, changeMainErr = dbhelper.ChangeMain(cluster.oldMain.Conn, dbhelper.ChangeMainOpt{
				Host:        cluster.main.Host,
				Port:        cluster.main.Port,
				User:        cluster.rplUser,
				Password:    cluster.rplPass,
				Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
				Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
				Mode:        "MASTER_AUTO_POSITION",
				SSL:         cluster.Conf.ReplicationSSL,
				Channel:     cluster.Conf.MainConn,
				IsDelayed:   cluster.oldMain.IsDelayed,
				Delay:       strconv.Itoa(cluster.oldMain.ClusterGroup.Conf.HostsDelayedTime),
				PostgressDB: cluster.main.PostgressDB,
			}, cluster.oldMain.DBVersion)
			cluster.LogSQL(logs, changeMainErr, cluster.oldMain.URL, "MainFailover", LvlErr, "Change main failed on old main %s", logs)
			logs, err = cluster.oldMain.StartSubordinate()
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Start subordinate failed on old main,%s reason:  %s ", cluster.oldMain.URL, err)
		} else if cluster.Conf.MxsBinlogOn == false {
			cluster.LogPrintf(LvlInfo, "Doing MariaDB GTID switch of the old main")
			// current pos is needed on old main as writes diverges from subordinate pos
			// if gtid_subordinate_pos was forced use subordinate_pos : positional to GTID promotion
			if one_shoot_subordinate_pos {
				logs, changeMainErr = dbhelper.ChangeMain(cluster.oldMain.Conn, dbhelper.ChangeMainOpt{
					Host:        cluster.main.Host,
					Port:        cluster.main.Port,
					User:        cluster.rplUser,
					Password:    cluster.rplPass,
					Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
					Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
					Mode:        "SLAVE_POS",
					SSL:         cluster.Conf.ReplicationSSL,
					Channel:     cluster.Conf.MainConn,
					IsDelayed:   cluster.oldMain.IsDelayed,
					Delay:       strconv.Itoa(cluster.oldMain.ClusterGroup.Conf.HostsDelayedTime),
					PostgressDB: cluster.main.PostgressDB,
				}, cluster.oldMain.DBVersion)
			} else {
				logs, changeMainErr = dbhelper.ChangeMain(cluster.oldMain.Conn, dbhelper.ChangeMainOpt{
					Host:        cluster.main.Host,
					Port:        cluster.main.Port,
					User:        cluster.rplUser,
					Password:    cluster.rplPass,
					Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
					Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
					Mode:        "CURRENT_POS",
					SSL:         cluster.Conf.ReplicationSSL,
					Channel:     cluster.Conf.MainConn,
					IsDelayed:   cluster.oldMain.IsDelayed,
					Delay:       strconv.Itoa(cluster.oldMain.ClusterGroup.Conf.HostsDelayedTime),
					PostgressDB: cluster.main.PostgressDB,
				}, cluster.oldMain.DBVersion)
			}
			cluster.LogSQL(logs, changeMainErr, cluster.oldMain.URL, "MainFailover", LvlErr, "Change main failed on old main %s", changeMainErr)
			logs, err = cluster.oldMain.StartSubordinate()
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Start subordinate failed on old main,%s reason:  %s ", cluster.oldMain.URL, err)
		} else {
			// Don't start subordinate until the relay as been point to new main
			cluster.LogPrintf(LvlInfo, "Pointing old main to relay server")
			if relaymain.MxsHaveGtid {
				logs, err = dbhelper.ChangeMain(cluster.oldMain.Conn, dbhelper.ChangeMainOpt{
					Host:        relaymain.Host,
					Port:        relaymain.Port,
					User:        cluster.rplUser,
					Password:    cluster.rplPass,
					Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
					Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
					Mode:        "SLAVE_POS",
					SSL:         cluster.Conf.ReplicationSSL,
					Channel:     cluster.Conf.MainConn,
					IsDelayed:   cluster.oldMain.IsDelayed,
					Delay:       strconv.Itoa(cluster.oldMain.ClusterGroup.Conf.HostsDelayedTime),
					PostgressDB: relaymain.PostgressDB,
				}, cluster.oldMain.DBVersion)
			} else {
				logs, err = dbhelper.ChangeMain(cluster.oldMain.Conn, dbhelper.ChangeMainOpt{
					Host:        relaymain.Host,
					Port:        relaymain.Port,
					User:        cluster.rplUser,
					Password:    cluster.rplPass,
					Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
					Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
					Mode:        "POSITIONAL",
					Logfile:     crash.FailoverMainLogFile,
					Logpos:      crash.FailoverMainLogPos,
					SSL:         cluster.Conf.ReplicationSSL,
					Channel:     cluster.Conf.MainConn,
					IsDelayed:   cluster.oldMain.IsDelayed,
					Delay:       strconv.Itoa(cluster.oldMain.ClusterGroup.Conf.HostsDelayedTime),
					PostgressDB: relaymain.PostgressDB,
				}, cluster.oldMain.DBVersion)
			}
		}
		cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Change main failed on old main %s", err)

		if cluster.Conf.ReadOnly {
			logs, err = dbhelper.SetReadOnly(cluster.oldMain.Conn, true)
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not set old main as read-only, %s", err)

		} else {
			logs, err = dbhelper.SetReadOnly(cluster.oldMain.Conn, false)
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not set old main as read-write, %s", err)
		}
		if cluster.Conf.SwitchDecreaseMaxConn {

			logs, err := dbhelper.SetMaxConnections(cluster.oldMain.Conn, cluster.oldMain.maxConn, cluster.oldMain.DBVersion)
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not set max connection, %s", err)

		}
		// Add the old main to the subordinates list

		cluster.oldMain.State = stateSubordinate
		if cluster.Conf.MultiMain == false {
			cluster.subordinates = append(cluster.subordinates, cluster.oldMain)
		}
	}

	// ********
	// Phase 5: Switch subordinates to new main
	// ********

	cluster.LogPrintf(LvlInfo, "Switching other subordinates to the new main")
	for _, sl := range cluster.subordinates {
		// Don't switch if subordinate was the old main or is in a multiple main setup or with relay server.
		if sl.URL == cluster.oldMain.URL || sl.State == stateMain || (sl.IsRelay == false && cluster.Conf.MxsBinlogOn == true) {
			continue
		}
		// maxscale is in the list of subordinate

		if fail == false && cluster.Conf.MxsBinlogOn == false && cluster.Conf.SwitchSubordinateWaitCatch {
			sl.WaitSyncToMain(cluster.oldMain)
		}
		cluster.LogPrintf(LvlInfo, "Change main on subordinate %s", sl.URL)
		logs, err = sl.StopSubordinate()
		cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not stop subordinate on server %s, %s", sl.URL, err)
		if fail == false && cluster.Conf.MxsBinlogOn == false && cluster.Conf.SwitchSubordinateWaitCatch {
			if cluster.Conf.FailForceGtid && sl.DBVersion.IsMariaDB() {
				logs, err := dbhelper.SetGTIDSubordinatePos(sl.Conn, cluster.oldMain.GTIDBinlogPos.Sprint())
				cluster.LogSQL(logs, err, sl.URL, "MainFailover", LvlErr, "Could not set gtid_subordinate_pos on subordinate %s, %s", sl.URL, err)
			}
		}
		hasMyGTID := cluster.main.HasMySQLGTID()

		var changeMainErr error

		// Not MariaDB and not using MySQL GTID, 2.0 stop doing any thing until pseudo GTID
		if sl.DBVersion.IsMariaDB() == false && hasMyGTID == false {

			if cluster.Conf.AutorejoinSubordinatePositionalHeartbeat == true {

				pseudoGTID, logs, err := sl.GetLastPseudoGTID()
				cluster.LogSQL(logs, err, sl.URL, "MainFailover", LvlErr, "Could not get pseudoGTID on subordinate %s, %s", sl.URL, err)
				cluster.LogPrintf(LvlInfo, "Found pseudoGTID %s", pseudoGTID)
				slFile, slPos, logs, err := sl.GetBinlogPosFromPseudoGTID(pseudoGTID)
				cluster.LogSQL(logs, err, sl.URL, "MainFailover", LvlErr, "Could not find pseudoGTID in subordinate %s, %s", sl.URL, err)
				cluster.LogPrintf(LvlInfo, "Found Coordinates on subordinate %s, %s", slFile, slPos)
				slSkip, logs, err := sl.GetNumberOfEventsAfterPos(slFile, slPos)
				cluster.LogSQL(logs, err, sl.URL, "MainFailover", LvlErr, "Could not find number of events after pseudoGTID in subordinate %s, %s", sl.URL, err)
				cluster.LogPrintf(LvlInfo, "Found %d events to skip after coordinates on subordinate %s,%s", slSkip, slFile, slPos)

				mFile, mPos, logs, err := cluster.main.GetBinlogPosFromPseudoGTID(pseudoGTID)
				cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlErr, "Could not find pseudoGTID in main %s, %s", cluster.main.URL, err)
				cluster.LogPrintf(LvlInfo, "Found coordinate on main %s ,%s", mFile, mPos)
				mFile, mPos, logs, err = cluster.main.GetBinlogPosAfterSkipNumberOfEvents(mFile, mPos, slSkip)
				cluster.LogSQL(logs, err, cluster.main.URL, "MainFailover", LvlErr, "Could not skip event after pseudoGTID in main %s, %s", cluster.main.URL, err)
				cluster.LogPrintf(LvlInfo, "Found skip coordinate on main %s, %s", mFile, mPos)

				cluster.LogPrintf(LvlInfo, "Doing Positional switch of subordinate %s", sl.URL)
				logs, changeMainErr = dbhelper.ChangeMain(sl.Conn, dbhelper.ChangeMainOpt{
					Host:        cluster.main.Host,
					Port:        cluster.main.Port,
					User:        cluster.rplUser,
					Password:    cluster.rplPass,
					Logfile:     mFile,
					Logpos:      mPos,
					Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
					Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
					Mode:        "POSITIONAL",
					SSL:         cluster.Conf.ReplicationSSL,
					Channel:     cluster.Conf.MainConn,
					IsDelayed:   sl.IsDelayed,
					Delay:       strconv.Itoa(sl.ClusterGroup.Conf.HostsDelayedTime),
					PostgressDB: cluster.main.PostgressDB,
				}, sl.DBVersion)
			} else {
				sl.SetMaintenance()
			}
			// do nothing stay connected to dead main proceed with relay fix later

		} else if cluster.oldMain.DBVersion.IsMySQLOrPerconaGreater57() && hasMyGTID == true {
			logs, changeMainErr = dbhelper.ChangeMain(sl.Conn, dbhelper.ChangeMainOpt{
				Host:        cluster.main.Host,
				Port:        cluster.main.Port,
				User:        cluster.rplUser,
				Password:    cluster.rplPass,
				Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
				Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
				Mode:        "MASTER_AUTO_POSITION",
				SSL:         cluster.Conf.ReplicationSSL,
				Channel:     cluster.Conf.MainConn,
				IsDelayed:   sl.IsDelayed,
				Delay:       strconv.Itoa(sl.ClusterGroup.Conf.HostsDelayedTime),
				PostgressDB: cluster.main.PostgressDB,
			}, sl.DBVersion)
		} else if cluster.Conf.MxsBinlogOn == false {
			//MariaDB all cases use GTID

			logs, changeMainErr = dbhelper.ChangeMain(sl.Conn, dbhelper.ChangeMainOpt{
				Host:        cluster.main.Host,
				Port:        cluster.main.Port,
				User:        cluster.rplUser,
				Password:    cluster.rplPass,
				Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
				Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
				Mode:        "SLAVE_POS",
				SSL:         cluster.Conf.ReplicationSSL,
				Channel:     cluster.Conf.MainConn,
				IsDelayed:   sl.IsDelayed,
				Delay:       strconv.Itoa(sl.ClusterGroup.Conf.HostsDelayedTime),
				PostgressDB: cluster.main.PostgressDB,
			}, sl.DBVersion)
		} else { // We deduct we are in maxscale binlog server , but can have support for GTID or not

			cluster.LogPrintf(LvlInfo, "Pointing relay to the new main: %s:%s", cluster.main.Host, cluster.main.Port)
			if sl.MxsHaveGtid {
				logs, changeMainErr = dbhelper.ChangeMain(sl.Conn, dbhelper.ChangeMainOpt{
					Host:        cluster.main.Host,
					Port:        cluster.main.Port,
					User:        cluster.rplUser,
					Password:    cluster.rplPass,
					Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
					Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
					Mode:        "SLAVE_POS",
					SSL:         cluster.Conf.ReplicationSSL,
					Channel:     cluster.Conf.MainConn,
					IsDelayed:   sl.IsDelayed,
					Delay:       strconv.Itoa(sl.ClusterGroup.Conf.HostsDelayedTime),
					PostgressDB: cluster.main.PostgressDB,
				}, sl.DBVersion)
			} else {
				logs, changeMainErr = dbhelper.ChangeMain(sl.Conn, dbhelper.ChangeMainOpt{
					Host:      cluster.main.Host,
					Port:      cluster.main.Port,
					User:      cluster.rplUser,
					Password:  cluster.rplPass,
					Retry:     strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
					Heartbeat: strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
					Mode:      "MXS",
					SSL:       cluster.Conf.ReplicationSSL,
				}, sl.DBVersion)
			}
		}
		cluster.LogSQL(logs, changeMainErr, sl.URL, "MainFailover", LvlErr, "Change main failed on subordinate %s, %s", sl.URL, changeMainErr)
		logs, err = sl.StartSubordinate()
		cluster.LogSQL(logs, err, sl.URL, "MainFailover", LvlErr, "Could not start subordinate on server %s, %s", sl.URL, err)
		// now start the old main as relay is ready
		if cluster.Conf.MxsBinlogOn && fail == false {
			cluster.LogPrintf(LvlInfo, "Restarting old main replication relay server ready")
			cluster.oldMain.StartSubordinate()
		}
		if cluster.Conf.ReadOnly && cluster.Conf.MxsBinlogOn == false && !cluster.IsInIgnoredReadonly(sl) {
			logs, err = sl.SetReadOnly()
			cluster.LogSQL(logs, err, sl.URL, "MainFailover", LvlErr, "Could not set subordinate %s as read-only, %s", sl.URL, err)
		} else {
			if cluster.Conf.MxsBinlogOn == false {
				err = sl.SetReadWrite()
				if err != nil {
					cluster.LogPrintf(LvlErr, "Could not remove subordinate %s as read-only, %s", sl.URL, err)
				}
			}
		}
	}
	// if consul or internal proxy need to adapt read only route to new subordinates
	cluster.backendStateChangeProxies()

	if fail == true && cluster.Conf.PrefMain != cluster.oldMain.URL && cluster.main.URL != cluster.Conf.PrefMain && cluster.Conf.PrefMain != "" {
		prm := cluster.foundPreferedMain(cluster.subordinates)
		if prm != nil {
			cluster.LogPrintf(LvlInfo, "Not on Preferred Main after failover")
			cluster.MainFailover(false)
		}
	}

	cluster.LogPrintf(LvlInfo, "Main switch on %s complete", cluster.main.URL)
	cluster.main.FailCount = 0
	if fail == true {
		cluster.FailoverCtr++
		cluster.FailoverTs = time.Now().Unix()
	}
	cluster.sme.RemoveFailoverState()
	return true
}

// Returns a candidate from a list of subordinates. If there's only one subordinate it will be the de facto candidate.
func (cluster *Cluster) electSwitchoverCandidate(l []*ServerMonitor, forcingLog bool) int {
	ll := len(l)
	seqList := make([]uint64, ll)
	posList := make([]uint64, ll)
	hipos := 0
	hiseq := 0
	var max uint64
	var maxpos uint64

	for i, sl := range l {

		/* If server is in the ignore list, do not elect it in switchover */
		if sl.IsIgnored() {
			cluster.sme.AddState("ERR00037", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00037"], sl.URL), ServerUrl: sl.URL, ErrFrom: "CHECK"})
			continue
		}
		//Need comment//
		if sl.IsRelay {
			cluster.sme.AddState("ERR00036", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00036"], sl.URL), ServerUrl: sl.URL, ErrFrom: "CHECK"})
			continue
		}
		if cluster.Conf.MultiMain == true && sl.State == stateMain {
			cluster.sme.AddState("ERR00035", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00035"], sl.URL), ServerUrl: sl.URL, ErrFrom: "CHECK"})
			continue
		}

		// The tests below should run only in case of a switchover as they require the main to be up.

		if cluster.isSubordinateElectableForSwitchover(sl, forcingLog) == false {
			cluster.sme.AddState("ERR00034", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00034"], sl.URL), ServerUrl: sl.URL, ErrFrom: "CHECK"})
			continue
		}
		/* binlog + ping  */
		if cluster.isSubordinateElectable(sl, forcingLog) == false {
			cluster.sme.AddState("ERR00039", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00039"], sl.URL), ServerUrl: sl.URL, ErrFrom: "CHECK"})
			continue
		}

		/* Rig the election if the examined subordinate is preferred candidate main in switchover */
		if sl.URL == cluster.Conf.PrefMain {
			if (cluster.Conf.LogLevel > 1 || forcingLog) && cluster.IsInFailover() {
				cluster.LogPrintf(LvlDbg, "Election rig: %s elected as preferred main", sl.URL)
			}
			return i
		}
		ss, errss := sl.GetSubordinateStatus(sl.ReplicationSourceName)
		// not a subordinate
		if errss != nil && cluster.Conf.FailRestartUnsafe == false {
			cluster.sme.AddState("ERR00033", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00033"], sl.URL), ServerUrl: sl.URL, ErrFrom: "CHECK"})
			continue
		}
		// Fake position if none as new subordinate
		filepos := "1"
		logfile := "main.000001"
		if errss == nil {
			filepos = ss.ReadMainLogPos.String
			logfile = ss.MainLogFile.String
		}
		if strings.Contains(logfile, ".") == false {
			continue
		}
		for len(filepos) < 12 {
			filepos = "0" + filepos
		}

		pos := strings.Split(logfile, ".")[1] + filepos
		binlogposreach, _ := strconv.ParseUint(pos, 10, 64)

		posList[i] = binlogposreach

		seqnos := gtid.NewList("1-1-1").GetSeqNos()

		if errss == nil {
			if cluster.main.State != stateFailed {
				seqnos = sl.SubordinateGtid.GetSeqNos()
			} else {
				seqnos = gtid.NewList(ss.GtidIOPos.String).GetSeqNos()
			}
		}

		for _, v := range seqnos {
			seqList[i] += v
		}
		if seqList[i] > max {
			max = seqList[i]
			hiseq = i
		}
		if posList[i] > maxpos {
			maxpos = posList[i]
			hipos = i
		}

	} //end loop all subordinates
	if max > 0 {
		/* Return key of subordinate with the highest seqno. */
		return hiseq
	}
	if maxpos > 0 {
		/* Return key of subordinate with the highest pos. */
		return hipos
	}
	return -1
}

func (cluster *Cluster) electFailoverCandidate(l []*ServerMonitor, forcingLog bool) int {
	//Found the most uptodate and look after a possibility to failover on it
	ll := len(l)
	seqList := make([]uint64, ll)
	posList := make([]uint64, ll)

	var maxseq uint64
	var maxpos uint64
	type Trackpos struct {
		URL                string
		Indice             int
		Pos                uint64
		Seq                uint64
		Prefered           bool
		Ignoredconf        bool
		Ignoredrelay       bool
		Ignoredmultimain bool
		Ignoredreplication bool
		Weight             uint
	}
	trackposList := make([]Trackpos, ll)
	for i, sl := range l {
		trackposList[i].URL = sl.URL
		trackposList[i].Indice = i
		trackposList[i].Prefered = sl.IsPrefered()
		trackposList[i].Ignoredconf = sl.IsIgnored()
		trackposList[i].Ignoredrelay = sl.IsRelay

		//Need comment//
		if sl.IsRelay {
			cluster.sme.AddState("ERR00036", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00036"], sl.URL), ErrFrom: "CHECK", ServerUrl: sl.URL})
			continue
		}
		if cluster.Conf.MultiMain == true && sl.State == stateMain {
			cluster.sme.AddState("ERR00035", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00035"], sl.URL), ErrFrom: "CHECK", ServerUrl: sl.URL})
			trackposList[i].Ignoredmultimain = true
			continue
		}
		if cluster.GetTopology() == topoMultiMainWsrep && cluster.vmain != nil {
			if cluster.vmain.URL == sl.URL {

				continue
			} else if sl.State == stateWsrep {
				return i
			} else {
				continue
			}
		}

		ss, errss := sl.GetSubordinateStatus(sl.ReplicationSourceName)
		// not a subordinate
		if errss != nil && cluster.Conf.FailRestartUnsafe == false {
			cluster.sme.AddState("ERR00033", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00033"], sl.URL), ErrFrom: "CHECK", ServerUrl: sl.URL})
			trackposList[i].Ignoredreplication = true
			continue
		}
		trackposList[i].Ignoredreplication = !cluster.isSubordinateElectable(sl, false)
		// Fake position if none as new subordinate
		filepos := "1"
		logfile := "main.000001"
		if errss == nil {
			filepos = ss.ReadMainLogPos.String
			logfile = ss.MainLogFile.String
		}
		if strings.Contains(logfile, ".") == false {
			continue
		}
		for len(filepos) < 12 {
			filepos = "0" + filepos
		}

		pos := strings.Split(logfile, ".")[1] + filepos
		binlogposreach, _ := strconv.ParseUint(pos, 10, 64)

		posList[i] = binlogposreach
		trackposList[i].Pos = binlogposreach

		seqnos := gtid.NewList("1-1-1").GetSeqNos()

		if errss == nil {
			if cluster.main.State != stateFailed {
				// Need MySQL GTID support
				seqnos = sl.SubordinateGtid.GetSeqNos()
			} else {
				seqnos = gtid.NewList(ss.GtidIOPos.String).GetSeqNos()
			}
		}

		for _, v := range seqnos {
			seqList[i] += v
		}
		trackposList[i].Seq = seqList[i]
		if seqList[i] > maxseq {
			maxseq = seqList[i]

		}
		if posList[i] > maxpos {
			maxpos = posList[i]

		}

	} //end loop all subordinates
	sort.Slice(trackposList[:], func(i, j int) bool {
		return trackposList[i].Seq > trackposList[j].Seq
	})

	if forcingLog {
		data, _ := json.MarshalIndent(trackposList, "", "\t")
		cluster.LogPrintf(LvlInfo, "Election matrice: %s ", data)
	}

	if maxseq > 0 {
		/* Return key of subordinate with the highest seqno. */

		//send the prefered if equal max
		for _, p := range trackposList {
			if p.Seq == maxseq && p.Ignoredrelay == false && p.Ignoredmultimain == false && p.Ignoredreplication == false && p.Ignoredconf == false && p.Prefered == true {
				return p.Indice
			}
		}
		//send one with maxseq
		for _, p := range trackposList {
			if p.Seq == maxseq && p.Ignoredrelay == false && p.Ignoredmultimain == false && p.Ignoredreplication == false && p.Ignoredconf == false {
				return p.Indice
			}
		}
		//send one with maxseq but also ignored
		for _, p := range trackposList {
			if p.Seq == maxseq && p.Ignoredrelay == false && p.Ignoredmultimain == false && p.Ignoredreplication == false && p.Ignoredconf == true {
				if forcingLog {
					cluster.LogPrintf(LvlInfo, "Ignored server is the most up to date ")
				}
				return p.Indice
			}

		}

		if cluster.Conf.LogFailedElection {
			data, _ := json.MarshalIndent(trackposList, "", "\t")
			cluster.LogPrintf(LvlInfo, "Election matrice maxseq >0: %s ", data)
		}
		return -1
	}
	sort.Slice(trackposList[:], func(i, j int) bool {
		return trackposList[i].Pos > trackposList[j].Pos
	})
	if maxpos > 0 {
		/* Return key of subordinate with the highest pos. */
		for _, p := range trackposList {
			if p.Pos == maxpos && p.Ignoredrelay == false && p.Ignoredmultimain == false && p.Ignoredreplication == false && p.Ignoredconf == false && p.Prefered == true {
				return p.Indice
			}
		}
		//send one with maxpos
		for _, p := range trackposList {
			if p.Pos == maxpos && p.Ignoredrelay == false && p.Ignoredmultimain == false && p.Ignoredreplication == false && p.Ignoredconf == false {
				return p.Indice
			}
		}
		//send one with maxpos and ignored
		for _, p := range trackposList {
			if p.Pos == maxpos && p.Ignoredrelay == false && p.Ignoredmultimain == false && p.Ignoredreplication == false && p.Ignoredconf == true {
				if forcingLog {
					cluster.LogPrintf(LvlInfo, "Ignored server is the most up to date ")
				}
				return p.Indice
			}
		}
		if cluster.Conf.LogFailedElection {
			data, _ := json.MarshalIndent(trackposList, "", "\t")
			cluster.LogPrintf(LvlInfo, "Election matrice maxpos>0: %s ", data)
		}
		return -1
	}
	if cluster.Conf.LogFailedElection {
		data, _ := json.MarshalIndent(trackposList, "", "\t")
		cluster.LogPrintf(LvlInfo, "Election matrice: %s ", data)
	}
	return -1
}

func (cluster *Cluster) isSubordinateElectable(sl *ServerMonitor, forcingLog bool) bool {
	ss, err := sl.GetSubordinateStatus(sl.ReplicationSourceName)
	if err != nil {
		cluster.LogPrintf(LvlWarn, "Error in getting subordinate status in testing subordinate electable %s: %s  ", sl.URL, err)
		return false
	}
	/* binlog + ping  */
	if dbhelper.CheckSubordinatePrerequisites(sl.Conn, sl.Host, sl.DBVersion) == false {
		cluster.sme.AddState("ERR00040", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00040"], sl.URL), ErrFrom: "CHECK", ServerUrl: sl.URL})
		if cluster.Conf.LogLevel > 1 || forcingLog {
			cluster.LogPrintf(LvlWarn, "Subordinate %s does not ping or has no binlogs. Skipping", sl.URL)
		}
		return false
	}
	if sl.IsMaintenance {
		cluster.sme.AddState("ERR00047", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00047"], sl.URL), ErrFrom: "CHECK", ServerUrl: sl.URL})
		if cluster.Conf.LogLevel > 1 || forcingLog {
			cluster.LogPrintf(LvlWarn, "Subordinate %s is in maintenance. Skipping", sl.URL)
		}
		return false
	}

	if ss.SecondsBehindMain.Int64 > cluster.Conf.FailMaxDelay && cluster.Conf.FailMaxDelay != -1 && cluster.Conf.RplChecks == true {
		cluster.sme.AddState("ERR00041", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00041"], sl.URL, cluster.Conf.FailMaxDelay, ss.SecondsBehindMain.Int64), ErrFrom: "CHECK", ServerUrl: sl.URL})
		if cluster.Conf.LogLevel > 1 || forcingLog {
			cluster.LogPrintf(LvlWarn, "Unsafe failover condition. Subordinate %s has more than failover-max-delay %d seconds with replication delay %d. Skipping", sl.URL, cluster.Conf.FailMaxDelay, ss.SecondsBehindMain.Int64)
		}
		return false
	}
	if ss.SubordinateSQLRunning.String == "No" && cluster.Conf.RplChecks {
		cluster.sme.AddState("ERR00042", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00042"], sl.URL), ErrFrom: "CHECK", ServerUrl: sl.URL})
		if cluster.Conf.LogLevel > 1 || forcingLog {
			cluster.LogPrintf(LvlWarn, "Unsafe failover condition. Subordinate %s SQL Thread is stopped. Skipping", sl.URL)
		}
		return false
	}
	if sl.HaveSemiSync && sl.SemiSyncSubordinateStatus == false && cluster.Conf.FailSync && cluster.Conf.RplChecks {
		cluster.sme.AddState("ERR00043", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00043"], sl.URL), ErrFrom: "CHECK", ServerUrl: sl.URL})
		if cluster.Conf.LogLevel > 1 || forcingLog {
			cluster.LogPrintf(LvlWarn, "Semi-sync subordinate %s is out of sync. Skipping", sl.URL)
		}
		return false
	}
	if sl.IsIgnored() {
		if cluster.Conf.LogLevel > 1 || forcingLog {
			cluster.LogPrintf(LvlWarn, "Subordinate is in ignored list %s", sl.URL)
		}
		return false
	}
	return true
}

func (cluster *Cluster) foundPreferedMain(l []*ServerMonitor) *ServerMonitor {
	for _, sl := range l {
		if strings.Contains(cluster.Conf.PrefMain, sl.URL) && cluster.main.State != stateFailed {
			return sl
		}
	}
	return nil
}

func (cluster *Cluster) VMainFailover(fail bool) bool {

	cluster.sme.SetFailoverState()
	// Phase 1: Cleanup and election
	var err error
	cluster.oldMain = cluster.vmain
	if fail == false {
		cluster.LogPrintf(LvlInfo, "----------------------------------")
		cluster.LogPrintf(LvlInfo, "Starting virtual main switchover")
		cluster.LogPrintf(LvlInfo, "----------------------------------")
		cluster.LogPrintf(LvlInfo, "Checking long running updates on virtual main %d", cluster.Conf.SwitchWaitWrite)
		if cluster.vmain == nil {
			cluster.LogPrintf(LvlErr, "Cannot switchover without a virtual main")
			return false
		}
		if cluster.vmain.Conn == nil {
			cluster.LogPrintf(LvlErr, "Cannot switchover without a vmain connection")
			return false
		}
		qt, logs, err := dbhelper.CheckLongRunningWrites(cluster.vmain.Conn, cluster.Conf.SwitchWaitWrite)
		cluster.LogSQL(logs, err, cluster.vmain.URL, "MainFailover", LvlDbg, "CheckLongRunningWrites")
		if qt > 0 {
			cluster.LogPrintf(LvlErr, "Long updates running on virtual main. Cannot switchover")
			cluster.sme.RemoveFailoverState()
			return false
		}

		cluster.LogPrintf(LvlInfo, "Flushing tables on virtual main %s", cluster.vmain.URL)
		workerFlushTable := make(chan error, 1)

		go func() {
			var err2 error
			logs, err2 = dbhelper.FlushTablesNoLog(cluster.vmain.Conn)
			cluster.LogSQL(logs, err, cluster.vmain.URL, "MainFailover", LvlDbg, "FlushTablesNoLog")

			workerFlushTable <- err2
		}()
		select {
		case err = <-workerFlushTable:
			if err != nil {
				cluster.LogPrintf(LvlWarn, "Could not flush tables on main", err)
			}
		case <-time.After(time.Second * time.Duration(cluster.Conf.SwitchWaitTrx)):
			cluster.LogPrintf(LvlErr, "Long running trx on main at least %d, can not switchover ", cluster.Conf.SwitchWaitTrx)
			cluster.sme.RemoveFailoverState()
			return false
		}
		cluster.main = cluster.vmain
	} else {
		cluster.LogPrintf(LvlInfo, "-------------------------------")
		cluster.LogPrintf(LvlInfo, "Starting virtual main failover")
		cluster.LogPrintf(LvlInfo, "-------------------------------")
		cluster.oldMain = cluster.main
	}
	cluster.LogPrintf(LvlInfo, "Electing a new virtual main")
	for _, s := range cluster.subordinates {
		s.Refresh()
	}
	key := -1
	if cluster.GetTopology() != topoMultiMainWsrep {
		key = cluster.electVirtualCandidate(cluster.oldMain, true)
	} else {
		key = cluster.electFailoverCandidate(cluster.subordinates, true)
	}
	if key == -1 {
		cluster.LogPrintf(LvlErr, "No candidates found")
		cluster.sme.RemoveFailoverState()
		return false
	}
	cluster.LogPrintf(LvlInfo, "Server %s has been elected as a new main", cluster.subordinates[key].URL)

	// Shuffle the server list

	var skey int
	for k, server := range cluster.Servers {
		if cluster.subordinates[key].URL == server.URL {
			skey = k
			break
		}
	}
	cluster.vmain = cluster.Servers[skey]
	cluster.main = cluster.Servers[skey]
	// Call pre-failover script
	if cluster.Conf.PreScript != "" {
		cluster.LogPrintf(LvlInfo, "Calling pre-failover script")
		var out []byte
		out, err = exec.Command(cluster.Conf.PreScript, cluster.oldMain.Host, cluster.vmain.Host, cluster.oldMain.Port, cluster.vmain.Port, cluster.oldMain.MxsServerName, cluster.vmain.MxsServerName).CombinedOutput()
		if err != nil {
			cluster.LogPrintf(LvlErr, "%s", err)
		}
		cluster.LogPrintf(LvlInfo, "Pre-failover script complete:", string(out))
	}

	// Phase 2: Reject updates and sync subordinates on switchover
	if fail == false {
		if cluster.Conf.FailEventStatus {
			for _, v := range cluster.vmain.EventStatus {
				if v.Status == 3 {
					cluster.LogPrintf(LvlInfo, "Set DISABLE ON SLAVE for event %s %s on old main", v.Db, v.Name)
					logs, err := dbhelper.SetEventStatus(cluster.oldMain.Conn, v, 3)
					cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not Set DISABLE ON SLAVE for event %s %s on old main", v.Db, v.Name)
				}
			}
		}
		if cluster.Conf.FailEventScheduler {

			cluster.LogPrintf(LvlInfo, "Disable Event Scheduler on old main")
			logs, err := dbhelper.SetEventScheduler(cluster.oldMain.Conn, false, cluster.oldMain.DBVersion)
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not disable event scheduler on old main")
		}
		cluster.oldMain.freeze()
		cluster.LogPrintf(LvlInfo, "Rejecting updates on %s (old main)", cluster.oldMain.URL)
		logs, err := dbhelper.FlushTablesWithReadLock(cluster.oldMain.Conn, cluster.oldMain.DBVersion)
		cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not lock tables on %s (old main) %s", cluster.oldMain.URL, err)
	}

	// Failover
	if cluster.GetTopology() != topoMultiMainWsrep {
		// Sync candidate depending on the main status.
		// If it's a switchover, use MASTER_POS_WAIT to sync.
		// If it's a failover, wait for the SQL thread to read all relay logs.
		// If maxsclale we should wait for relay catch via old style
		crash := new(Crash)
		crash.URL = cluster.oldMain.URL
		crash.ElectedMainURL = cluster.main.URL

		cluster.LogPrintf(LvlInfo, "Waiting for candidate main to apply relay log")
		err = cluster.main.ReadAllRelayLogs()
		if err != nil {
			cluster.LogPrintf(LvlErr, "Error while reading relay logs on candidate: %s", err)
		}
		cluster.LogPrintf("INFO ", "Save replication status before electing")
		ms, err := cluster.main.GetSubordinateStatus(cluster.main.ReplicationSourceName)
		if err != nil {
			cluster.LogPrintf(LvlErr, "Faiover can not fetch replication info on new main: %s", err)
		}
		cluster.LogPrintf(LvlInfo, "main_log_file=%s", ms.MainLogFile.String)
		cluster.LogPrintf(LvlInfo, "main_log_pos=%s", ms.ReadMainLogPos.String)
		cluster.LogPrintf(LvlInfo, "Candidate was in sync=%t", cluster.main.SemiSyncSubordinateStatus)
		//		cluster.main.FailoverMainLogFile = cluster.main.MainLogFile
		//		cluster.main.FailoverMainLogPos = cluster.main.MainLogPos
		crash.FailoverMainLogFile = ms.MainLogFile.String
		crash.FailoverMainLogPos = ms.ReadMainLogPos.String
		if cluster.main.DBVersion.IsMariaDB() {
			if cluster.Conf.MxsBinlogOn {
				//	cluster.main.FailoverIOGtid = cluster.main.CurrentGtid
				crash.FailoverIOGtid = cluster.main.CurrentGtid
			} else {
				//	cluster.main.FailoverIOGtid = gtid.NewList(ms.GtidIOPos.String)
				crash.FailoverIOGtid = gtid.NewList(ms.GtidIOPos.String)
			}
		} else if cluster.main.DBVersion.IsMySQLOrPerconaGreater57() && cluster.main.HasGTIDReplication() {
			crash.FailoverIOGtid = gtid.NewMySQLList(ms.ExecutedGtidSet.String)
		}
		cluster.main.FailoverSemiSyncSubordinateStatus = cluster.main.SemiSyncSubordinateStatus
		crash.FailoverSemiSyncSubordinateStatus = cluster.main.SemiSyncSubordinateStatus
		cluster.Crashes = append(cluster.Crashes, crash)
		cluster.Save()
		t := time.Now()
		crash.Save(cluster.WorkingDir + "/failover." + t.Format("20060102150405") + ".json")
		crash.Purge(cluster.WorkingDir, cluster.Conf.FailoverLogFileKeep)
	}

	// Phase 3: Prepare new main

	// Call post-failover script before unlocking the old main.
	if cluster.Conf.PostScript != "" {
		cluster.LogPrintf(LvlInfo, "Calling post-failover script")
		var out []byte
		out, err = exec.Command(cluster.Conf.PostScript, cluster.oldMain.Host, cluster.main.Host, cluster.oldMain.Port, cluster.main.Port, cluster.oldMain.MxsServerName, cluster.main.MxsServerName).CombinedOutput()
		if err != nil {
			cluster.LogPrintf(LvlErr, "%s", err)
		}
		cluster.LogPrintf(LvlInfo, "Post-failover script complete", string(out))
	}
	cluster.failoverProxies()
	cluster.main.SetReadWrite()

	if err != nil {
		cluster.LogPrintf(LvlErr, "Could not set new main as read-write")
	}
	if cluster.Conf.FailEventScheduler {
		cluster.LogPrintf(LvlInfo, "Enable Event Scheduler on the new main")
		logs, err := dbhelper.SetEventScheduler(cluster.vmain.Conn, true, cluster.vmain.DBVersion)
		cluster.LogSQL(logs, err, cluster.vmain.URL, "MainFailover", LvlErr, "Could not enable event scheduler on the new main")
	}
	if cluster.Conf.FailEventStatus {
		for _, v := range cluster.main.EventStatus {
			if v.Status == 3 {
				cluster.LogPrintf(LvlInfo, "Set ENABLE for event %s %s on new main", v.Db, v.Name)
				logs, err := dbhelper.SetEventStatus(cluster.vmain.Conn, v, 1)
				cluster.LogSQL(logs, err, cluster.vmain.URL, "MainFailover", LvlErr, "Could not Set ENABLE for event %s %s on new main", v.Db, v.Name)
			}
		}
	}

	if fail == false {
		// Get latest GTID pos
		cluster.oldMain.Refresh()

		// ********
		// Phase 4: Demote old main to subordinate
		// ********
		cluster.LogPrintf(LvlInfo, "Switching old main as a subordinate")
		logs, err := dbhelper.UnlockTables(cluster.oldMain.Conn)
		cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not unlock tables on old main %s", err)

		if cluster.Conf.ReadOnly {

			logs, err = cluster.oldMain.SetReadOnly()
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not set old main as read-only, %s", err)

		} else {
			err = cluster.oldMain.SetReadWrite()
			if err != nil {
				cluster.LogPrintf(LvlErr, "Could not set old main as read-write, %s", err)
			}
		}
		if cluster.Conf.SwitchDecreaseMaxConn {
			logs, err := dbhelper.SetMaxConnections(cluster.oldMain.Conn, cluster.oldMain.maxConn, cluster.oldMain.DBVersion)
			cluster.LogSQL(logs, err, cluster.oldMain.URL, "MainFailover", LvlErr, "Could not set max connections on %s %s", cluster.oldMain.URL, err)
		}
		// Add the old main to the subordinates list
	}
	if cluster.GetTopology() == topoMultiMainRing {
		// ********
		// Phase 5: Closing loop
		// ********
		cluster.CloseRing(cluster.oldMain)
	}
	cluster.LogPrintf(LvlInfo, "Virtual Main switch on %s complete", cluster.vmain.URL)
	cluster.vmain.FailCount = 0
	if fail == true {
		cluster.FailoverCtr++
		cluster.FailoverTs = time.Now().Unix()
	}
	cluster.main = nil

	cluster.sme.RemoveFailoverState()
	return true
}

func (cluster *Cluster) electVirtualCandidate(oldMain *ServerMonitor, forcingLog bool) int {

	for i, sl := range cluster.Servers {
		/* If server is in the ignore list, do not elect it */
		if sl.IsIgnored() {
			cluster.sme.AddState("ERR00037", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00037"], sl.URL), ErrFrom: "CHECK"})
			if cluster.Conf.LogLevel > 1 || forcingLog {
				cluster.LogPrintf(LvlDbg, "%s is in the ignore list. Skipping", sl.URL)
			}
			continue
		}
		if sl.State != stateFailed && sl.ServerID != oldMain.ServerID {
			return i
		}

	}
	return -1
}

func (cluster *Cluster) GetRingChildServer(oldMain *ServerMonitor) *ServerMonitor {
	for _, s := range cluster.Servers {
		if s.ServerID != cluster.oldMain.ServerID {
			//cluster.LogPrintf(LvlDbg, "test %s failed %s", s.URL, cluster.oldMain.URL)
			main, err := cluster.GetMainFromReplication(s)
			if err == nil && main.ServerID == oldMain.ServerID {
				return s
			}
		}
	}
	return nil
}

func (cluster *Cluster) GetRingParentServer(oldMain *ServerMonitor) *ServerMonitor {
	ss, err := cluster.oldMain.GetSubordinateStatusLastSeen(cluster.oldMain.ReplicationSourceName)
	if err != nil {
		return nil
	}
	return cluster.GetServerFromURL(ss.MainHost.String + ":" + ss.MainPort.String)
}

func (cluster *Cluster) CloseRing(oldMain *ServerMonitor) error {
	cluster.LogPrintf(LvlInfo, "Closing ring around %s", cluster.oldMain.URL)
	child := cluster.GetRingChildServer(cluster.oldMain)
	if child == nil {
		return errors.New("Can't find child in ring")
	}
	cluster.LogPrintf(LvlInfo, "Child is %s", child.URL)
	parent := cluster.GetRingParentServer(oldMain)
	if parent == nil {
		return errors.New("Can't find parent in ring")
	}
	cluster.LogPrintf(LvlInfo, "Parent is %s", parent.URL)
	logs, err := child.StopSubordinate()
	cluster.LogSQL(logs, err, child.URL, "MainFailover", LvlErr, "Could not stop subordinate on server %s, %s", child.URL, err)

	hasMyGTID := parent.HasMySQLGTID()

	var changeMainErr error

	// Not MariaDB and not using MySQL GTID, 2.0 stop doing any thing until pseudo GTID
	if parent.DBVersion.IsMySQLOrPerconaGreater57() && hasMyGTID == true {
		logs, changeMainErr = dbhelper.ChangeMain(child.Conn, dbhelper.ChangeMainOpt{
			Host:        parent.Host,
			Port:        parent.Port,
			User:        cluster.rplUser,
			Password:    cluster.rplPass,
			Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
			Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
			Mode:        "",
			SSL:         cluster.Conf.ReplicationSSL,
			Channel:     cluster.Conf.MainConn,
			PostgressDB: parent.PostgressDB,
		}, child.DBVersion)
	} else {
		//MariaDB all cases use GTID

		logs, changeMainErr = dbhelper.ChangeMain(child.Conn, dbhelper.ChangeMainOpt{
			Host:        parent.Host,
			Port:        parent.Port,
			User:        cluster.rplUser,
			Password:    cluster.rplPass,
			Retry:       strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatRetry),
			Heartbeat:   strconv.Itoa(cluster.Conf.ForceSubordinateHeartbeatTime),
			Mode:        "SLAVE_POS",
			SSL:         cluster.Conf.ReplicationSSL,
			Channel:     cluster.Conf.MainConn,
			PostgressDB: parent.PostgressDB,
		}, child.DBVersion)
	}

	cluster.LogSQL(logs, changeMainErr, child.URL, "MainFailover", LvlErr, "Could not change mainon server %s, %s", child.URL, changeMainErr)

	logs, err = child.StartSubordinate()
	cluster.LogSQL(logs, err, child.URL, "MainFailover", LvlErr, "Could not start subordinate on server %s, %s", child.URL, err)

	return nil
}
