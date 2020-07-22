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

	"github.com/signal18/replication-manager/utils/dbhelper"
	"github.com/signal18/replication-manager/utils/misc"
	"github.com/signal18/replication-manager/utils/state"
)

// CheckMaxConnections Check 80% of max connection reach
func (server *ServerMonitor) CheckMaxConnections() {
	maxCx, _ := strconv.ParseInt(server.Variables["MAX_CONNECTIONS"], 10, 64)
	curCx, _ := strconv.ParseInt(server.Status["THREADS_CONNECTED"], 10, 64)
	if curCx > maxCx*80/100 {
		server.ClusterGroup.sme.AddState("ERR00076", state.State{ErrType: LvlWarn, ErrDesc: fmt.Sprintf(clusterError["ERR00076"], server.URL), ErrFrom: "MON", ServerUrl: server.URL})
	}
}

func (server *ServerMonitor) CheckVersion() {

	if server.DBVersion.IsMariaDB() && ((server.DBVersion.Major == 10 && server.DBVersion.Minor == 4 && server.DBVersion.Release < 12) || (server.DBVersion.Major == 10 && server.DBVersion.Minor == 5 && server.DBVersion.Release < 1)) {
		server.ClusterGroup.sme.AddState("WARN0099", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0099"], server.URL), ErrFrom: "MON", ServerUrl: server.URL})
	}

}

// CheckDisks check mariadb disk plugin ti see if it get free space
func (server *ServerMonitor) CheckDisks() {
	for _, d := range server.Disks {
		if d.Used/d.Total*100 > int32(server.ClusterGroup.Conf.MonitorDiskUsagePct) {
			server.ClusterGroup.sme.AddState("ERR00079", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00079"], server.URL), ErrFrom: "MON", ServerUrl: server.URL})
		}
	}
}

// CheckReplication Check replication health and return status string
func (server *ServerMonitor) CheckReplication() string {
	if server.ClusterGroup.sme.IsInFailover() {
		return "In Failover"
	}
	if server.HaveWsrep && !server.IsFailed() {
		if server.IsWsrepSync {
			server.State = stateWsrep
			return "Galera OK"
		} else if server.IsWsrepDonor {
			server.State = stateWsrepDonor
			return "Galera OK"
		} else {
			server.State = stateWsrepLate
			return "Galera Late"
		}
	}
	if (server.IsDown()) && server.IsSubordinate == false {

		return "Main OK"
	}

	if server.ClusterGroup.main != nil {
		if server.ServerID == server.ClusterGroup.main.ServerID {
			return "Main OK"
		}
	}
	if server.IsMaintenance {
		server.State = stateMaintenance
		return "Maintenance"
	}
	// when replication stopped Valid is null
	ss, err := server.GetSubordinateStatus(server.ReplicationSourceName)
	if err != nil {
		return "Not a subordinate"
	}
	if ss.SecondsBehindMain.Valid == false {

		//	log.Printf("replicationCheck %s %s", server.SQLThread, server.IOThread)
		if ss.SubordinateSQLRunning.String == "Yes" && ss.SubordinateIORunning.String == "No" {
			if server.IsRelay == false && server.IsMaxscale == false {
				server.State = stateSubordinateErr
			} else if server.IsRelay {
				server.State = stateRelayErr
			}
			return fmt.Sprintf("NOT OK, IO Stopped (%s)", ss.LastIOErrno.String)
		} else if ss.SubordinateSQLRunning.String == "No" && ss.SubordinateIORunning.String == "Yes" {
			if server.IsRelay == false && server.IsMaxscale == false {
				server.State = stateSubordinateErr
			} else if server.IsRelay {
				server.State = stateRelayErr
			}
			return fmt.Sprintf("NOT OK, SQL Stopped (%s)", ss.LastSQLErrno.String)
		} else if ss.SubordinateSQLRunning.String == "No" && ss.SubordinateIORunning.String == "No" {
			if server.IsRelay == false && server.IsMaxscale == false {
				server.State = stateSubordinateErr
			} else if server.IsRelay {
				server.State = stateRelayErr
			}
			return "NOT OK, ALL Stopped"
		} else if ss.SubordinateSQLRunning.String == "Connecting" {
			if server.IsRelay == false && server.IsMaxscale == false {
				server.State = stateSubordinate
			} else if server.IsRelay {
				server.State = stateRelay
			}
			return "NOT OK, IO Connecting"
		}

		if server.IsRelay == false && server.IsMaxscale == false {
			server.State = stateSubordinate
		} else if server.IsRelay {
			server.State = stateRelay
		}
		return "Running OK"
	}

	if ss.SecondsBehindMain.Int64 > 0 {
		if ss.SecondsBehindMain.Int64 > server.ClusterGroup.Conf.FailMaxDelay && server.ClusterGroup.Conf.RplChecks == true {
			if server.IsRelay == false && server.IsMaxscale == false {
				server.State = stateSubordinateLate
			} else if server.IsRelay {
				server.State = stateRelayLate
			}

		} else {
			if server.IsRelay == false && server.IsMaxscale == false {
				server.State = stateSubordinate
			} else if server.IsRelay {
				server.State = stateRelay
			}
		}
		return "Behind main"
	}
	if server.IsRelay == false && server.IsMaxscale == false {
		server.State = stateSubordinate
	} else if server.IsRelay {
		server.State = stateRelayLate
	}
	return "Running OK"
}

// CheckSubordinateSettings check subordinate variables & enforce if set
func (server *ServerMonitor) CheckSubordinateSettings() {
	sl := server
	if server.ClusterGroup.Conf.ForceSubordinateSemisync && sl.HaveSemiSync == false {
		server.ClusterGroup.LogPrintf("DEBUG", "Enforce semisync on subordinate %s", sl.URL)
		dbhelper.InstallSemiSync(sl.Conn)
	} else if sl.IsIgnored() == false && sl.HaveSemiSync == false {
		server.ClusterGroup.sme.AddState("WARN0048", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0048"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}

	if server.ClusterGroup.Conf.ForceBinlogRow && sl.HaveBinlogRow == false {
		// In non-multimain mode, enforce read-only flag if the option is set
		dbhelper.SetBinlogFormat(sl.Conn, "ROW")
		server.ClusterGroup.LogPrintf("INFO", "Enforce binlog format ROW on subordinate %s", sl.URL)
	} else if sl.IsIgnored() == false && sl.HaveBinlogRow == false && server.ClusterGroup.Conf.AutorejoinFlashback == true {
		server.ClusterGroup.sme.AddState("WARN0049", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0049"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}
	if server.ClusterGroup.Conf.ForceSubordinateReadOnly && sl.ReadOnly == "OFF" && !server.ClusterGroup.IsInIgnoredReadonly(server) {
		// In non-multimain mode, enforce read-only flag if the option is set
		sl.SetReadOnly()
		server.ClusterGroup.LogPrintf("INFO", "Enforce read only on subordinate %s", sl.URL)
	}
	if server.ClusterGroup.Conf.ForceSubordinateHeartbeat && sl.GetReplicationHearbeatPeriod() > 1 {
		dbhelper.SetSubordinateHeartbeat(sl.Conn, "1", server.ClusterGroup.Conf.MainConn, server.DBVersion)
		server.ClusterGroup.LogPrintf("INFO", "Enforce heartbeat to 1s on subordinate %s", sl.URL)
	} else if sl.IsIgnored() == false && sl.GetReplicationHearbeatPeriod() > 1 {
		server.ClusterGroup.sme.AddState("WARN0050", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0050"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}
	if server.ClusterGroup.Conf.ForceSubordinateGtid && sl.GetReplicationUsingGtid() == "No" {
		dbhelper.SetSubordinateGTIDMode(sl.Conn, "subordinate_pos", server.ClusterGroup.Conf.MainConn, server.DBVersion)
		server.ClusterGroup.LogPrintf("INFO", "Enforce GTID replication on subordinate %s", sl.URL)
	} else if sl.IsIgnored() == false && sl.GetReplicationUsingGtid() == "No" {
		server.ClusterGroup.sme.AddState("WARN0051", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0051"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}
	if server.ClusterGroup.Conf.ForceSubordinateGtidStrict && sl.IsReplicationUsingGtidStrict() == false {
		dbhelper.SetSubordinateGTIDModeStrict(sl.Conn, server.DBVersion)
		server.ClusterGroup.LogPrintf("INFO", "Enforce GTID strict mode on subordinate %s", sl.URL)
	} else if sl.IsIgnored() == false && sl.IsReplicationUsingGtidStrict() == false {
		server.ClusterGroup.sme.AddState("WARN0058", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0058"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}

	if server.ClusterGroup.Conf.ForceSyncInnoDB && sl.HaveInnodbTrxCommit == false {
		dbhelper.SetSyncInnodb(sl.Conn)
		server.ClusterGroup.LogPrintf("INFO", "Enforce InnoDB durability on subordinate %s", sl.URL)
	} else if sl.IsIgnored() == false && sl.HaveInnodbTrxCommit == false {
		server.ClusterGroup.sme.AddState("WARN0052", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0052"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}
	if server.ClusterGroup.Conf.ForceBinlogChecksum && sl.HaveChecksum == false {
		dbhelper.SetBinlogChecksum(sl.Conn)
		server.ClusterGroup.LogPrintf("INFO", "Enforce checksum on subordinate %s", sl.URL)
	} else if sl.IsIgnored() == false && sl.HaveChecksum == false {
		server.ClusterGroup.sme.AddState("WARN0053", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0053"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}
	if server.ClusterGroup.Conf.ForceBinlogSlowqueries && sl.HaveBinlogSlowqueries == false {
		dbhelper.SetBinlogSlowqueries(sl.Conn)
		server.ClusterGroup.LogPrintf("INFO", "Enforce log slow queries of replication on subordinate %s", sl.URL)
	} else if sl.IsIgnored() == false && sl.HaveBinlogSlowqueries == false {
		server.ClusterGroup.sme.AddState("WARN0054", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0054"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}
	if server.ClusterGroup.Conf.ForceBinlogAnnotate && sl.HaveBinlogAnnotate == false && server.IsMariaDB() {
		dbhelper.SetBinlogAnnotate(sl.Conn)
		server.ClusterGroup.LogPrintf("INFO", "Enforce annotate on subordinate %s", sl.URL)
	} else if sl.IsIgnored() == false && sl.HaveBinlogAnnotate == false && server.IsMariaDB() {
		server.ClusterGroup.sme.AddState("WARN0055", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0055"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}

	if server.ClusterGroup.Conf.ForceBinlogCompress && sl.HaveBinlogCompress == false && sl.DBVersion.IsMariaDB() && sl.DBVersion.Major >= 10 && sl.DBVersion.Minor >= 2 {
		dbhelper.SetBinlogCompress(sl.Conn)
		server.ClusterGroup.LogPrintf("INFO", "Enforce binlog compression on subordinate %s", sl.URL)
	} else if sl.IsIgnored() == false && sl.HaveBinlogCompress == false && sl.DBVersion.IsMariaDB() && sl.DBVersion.Major >= 10 && sl.DBVersion.Minor >= 2 {
		server.ClusterGroup.sme.AddState("WARN0056", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0056"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}
	if sl.IsIgnored() == false && sl.HaveBinlogSubordinateUpdates == false {
		server.ClusterGroup.sme.AddState("WARN0057", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0057"], sl.URL), ErrFrom: "TOPO", ServerUrl: sl.URL})
	}

	if server.IsAcid() == false && server.ClusterGroup.IsDiscovered() {
		server.ClusterGroup.SetState("WARN0007", state.State{ErrType: "WARNING", ErrDesc: "At least one server is not ACID-compliant. Please make sure that sync_binlog and innodb_flush_log_at_trx_commit are set to 1", ErrFrom: "CONF", ServerUrl: sl.URL})
	}

}

// CheckMainSettings check main variables & enforce if set
func (server *ServerMonitor) CheckMainSettings() {
	if server.ClusterGroup.Conf.ForceSubordinateSemisync && server.HaveSemiSync == false {
		server.ClusterGroup.LogPrintf("INFO", "Enforce semisync on Main %s", server.URL)
		dbhelper.InstallSemiSync(server.Conn)
	} else if server.HaveSemiSync == false {
		server.ClusterGroup.sme.AddState("WARN0060", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0060"], server.URL), ErrFrom: "TOPO", ServerUrl: server.URL})
	}
	if server.ClusterGroup.Conf.ForceBinlogRow && server.HaveBinlogRow == false {
		dbhelper.SetBinlogFormat(server.Conn, "ROW")
		server.ClusterGroup.LogPrintf("INFO", "Enforce binlog format ROW on Main %s", server.URL)
	} else if server.HaveBinlogRow == false && server.ClusterGroup.Conf.AutorejoinFlashback == true {
		server.ClusterGroup.sme.AddState("WARN0061", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0061"], server.URL), ErrFrom: "TOPO", ServerUrl: server.URL})
	}
	if server.ClusterGroup.Conf.ForceSyncBinlog && server.HaveBinlogSync == false {
		dbhelper.SetSyncBinlog(server.Conn)
		server.ClusterGroup.LogPrintf("INFO", "Enforce sync binlog on Main %s", server.URL)
	} else if server.HaveBinlogSync == false {
		server.ClusterGroup.sme.AddState("WARN0062", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0062"], server.URL), ErrFrom: "TOPO", ServerUrl: server.URL})
	}
	if server.ClusterGroup.Conf.ForceSyncInnoDB && server.HaveBinlogSync == false {
		dbhelper.SetSyncInnodb(server.Conn)
		server.ClusterGroup.LogPrintf("INFO", "Enforce innodb durability on Main %s", server.URL)
	} else if server.HaveBinlogSync == false {
		server.ClusterGroup.sme.AddState("WARN0064", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0064"], server.URL), ErrFrom: "TOPO", ServerUrl: server.URL})
	}
	if server.ClusterGroup.Conf.ForceBinlogAnnotate && server.HaveBinlogAnnotate == false && server.IsMariaDB() {
		dbhelper.SetBinlogAnnotate(server.Conn)
		server.ClusterGroup.LogPrintf("INFO", "Enforce binlog annotate on main %s", server.URL)
	} else if server.HaveBinlogAnnotate == false && server.IsMariaDB() {
		server.ClusterGroup.sme.AddState("WARN0067", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0067"], server.URL), ErrFrom: "TOPO", ServerUrl: server.URL})
	}
	if server.ClusterGroup.Conf.ForceBinlogChecksum && server.HaveChecksum == false {
		dbhelper.SetBinlogChecksum(server.Conn)
		server.ClusterGroup.LogPrintf("INFO", "Enforce ckecksum annotate on main %s", server.URL)
	} else if server.HaveChecksum == false {
		server.ClusterGroup.sme.AddState("WARN0065", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0065"], server.URL), ErrFrom: "TOPO", ServerUrl: server.URL})
	}
	if server.ClusterGroup.Conf.ForceBinlogCompress && server.HaveBinlogCompress == false && server.IsMariaDB() && server.DBVersion.Major >= 10 && server.DBVersion.Minor >= 2 {
		dbhelper.SetBinlogCompress(server.Conn)
		server.ClusterGroup.LogPrintf("INFO", "Enforce binlog compression on main %s", server.URL)
	} else if server.HaveBinlogCompress == false && server.DBVersion.IsMariaDB() && server.DBVersion.Major >= 10 && server.DBVersion.Minor >= 2 {
		server.ClusterGroup.sme.AddState("WARN0068", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0068"], server.URL), ErrFrom: "TOPO", ServerUrl: server.URL})
	}
	if server.HaveBinlogSubordinateUpdates == false {
		server.ClusterGroup.sme.AddState("WARN0069", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0069"], server.URL), ErrFrom: "TOPO", ServerUrl: server.URL})
	}
	if server.HaveGtidStrictMode == false {
		server.ClusterGroup.sme.AddState("WARN0070", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0070"], server.URL), ErrFrom: "TOPO", ServerUrl: server.URL})
	}
	if server.IsAcid() == false && server.ClusterGroup.IsDiscovered() {
		server.ClusterGroup.SetState("WARN0007", state.State{ErrType: "WARNING", ErrDesc: "At least one server is not ACID-compliant. Please make sure that sync_binlog and innodb_flush_log_at_trx_commit are set to 1", ErrFrom: "CONF", ServerUrl: server.URL})
	}
}

// CheckSubordinateSameMainGrants check same serers grants as the main
func (server *ServerMonitor) CheckSubordinateSameMainGrants() bool {
	if server.ClusterGroup.GetMain() == nil || server.IsIgnored() || server.ClusterGroup.Conf.CheckGrants == false {
		return true
	}
	for _, user := range server.ClusterGroup.GetMain().Users {
		if _, ok := server.Users["'"+user.User+"'@'"+user.Host+"'"]; !ok {
			server.ClusterGroup.sme.AddState("ERR00056", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00056"], fmt.Sprintf("'%s'@'%s'", user.User, user.Host), server.URL), ErrFrom: "TOPO", ServerUrl: server.URL})
			return false
		}
	}
	return true
}

// CheckPrivileges replication manager user privileges on live servers
func (server *ServerMonitor) CheckPrivileges() {
	if server.ClusterGroup.Conf.LogLevel > 2 {
		server.ClusterGroup.LogPrintf(LvlDbg, "Privilege check on %s", server.URL)
	}
	if server.State != "" && !server.IsDown() && server.IsRelay == false {
		myhost, logs, err := dbhelper.GetHostFromConnection(server.Conn, server.ClusterGroup.dbUser, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlErr, "Check Privileges can't get hostname from server %s connection on %s: %s", server.State, server.URL, err)
		myip, err := misc.GetIPSafe(misc.Unbracket(myhost))
		if server.ClusterGroup.Conf.LogLevel > 2 {
			server.ClusterGroup.LogPrintf(LvlDbg, "Client connection found on server %s with IP %s for host %s", server.URL, myip, myhost)
		}
		if err != nil {
			server.ClusterGroup.SetState("ERR00078", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00005"], server.ClusterGroup.dbUser, server.URL, myhost, err), ErrFrom: "CONF", ServerUrl: server.URL})
		} else {
			priv, logs, err := dbhelper.GetPrivileges(server.Conn, server.ClusterGroup.dbUser, server.ClusterGroup.repmgrHostname, myip, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, fmt.Sprintf(clusterError["ERR00005"], server.ClusterGroup.dbUser, server.ClusterGroup.repmgrHostname, err))
			if err != nil {
				server.ClusterGroup.SetState("ERR00005", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00005"], server.ClusterGroup.dbUser, server.ClusterGroup.repmgrHostname, err), ErrFrom: "CONF", ServerUrl: server.URL})
			}
			if priv.Repl_client_priv == "N" {
				server.ClusterGroup.SetState("ERR00006", state.State{ErrType: "ERROR", ErrDesc: clusterError["ERR00006"], ErrFrom: "CONF", ServerUrl: server.URL})
			}
			if priv.Super_priv == "N" {
				server.ClusterGroup.SetState("ERR00008", state.State{ErrType: "ERROR", ErrDesc: clusterError["ERR00008"], ErrFrom: "CONF", ServerUrl: server.URL})
			}
			if priv.Reload_priv == "N" {
				server.ClusterGroup.SetState("ERR00009", state.State{ErrType: "ERROR", ErrDesc: clusterError["ERR00009"], ErrFrom: "CONF", ServerUrl: server.URL})
			}
		}
		// Check replication user has correct privs.
		for _, sv2 := range server.ClusterGroup.Servers {
			if sv2.URL != server.URL && sv2.IsRelay == false && !sv2.IsDown() {
				rplhost, _ := misc.GetIPSafe(misc.Unbracket(sv2.Host))
				rpriv, logs, err := dbhelper.GetPrivileges(server.Conn, server.ClusterGroup.rplUser, sv2.Host, rplhost, server.DBVersion)
				server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, fmt.Sprintf(clusterError["ERR00015"], server.ClusterGroup.rplUser, sv2.URL, err))
				if err != nil {
					server.ClusterGroup.SetState("ERR00015", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00015"], server.ClusterGroup.rplUser, sv2.URL, err), ErrFrom: "CONF", ServerUrl: sv2.URL})
				}
				if rpriv.Repl_subordinate_priv == "N" {
					server.ClusterGroup.SetState("ERR00007", state.State{ErrType: "ERROR", ErrDesc: clusterError["ERR00007"], ErrFrom: "CONF", ServerUrl: sv2.URL})
				}
			}
		}
	}
}
