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
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/signal18/replication-manager/utils/dbhelper"
	"github.com/signal18/replication-manager/utils/misc"
	"github.com/signal18/replication-manager/utils/state"
)

func (server *ServerMonitor) RejoinLoop() error {
	server.ClusterGroup.LogPrintf("INFO", "rejoin %s to the loop", server.URL)
	child := server.GetSibling()
	if child == nil {
		return errors.New("Could not found sibling subordinate")
	}
	child.StopSubordinate()
	child.SetReplicationGTIDSubordinatePosFromServer(server)
	child.StartSubordinate()
	return nil
}

// RejoinMain a server that just show up without subordinate status
func (server *ServerMonitor) RejoinMain() error {
	// Check if main exists in topology before rejoining.
	if server.ClusterGroup.sme.IsInFailover() {
		server.ClusterGroup.rejoinCond.Send <- true
		return nil
	}
	if server.ClusterGroup.Conf.LogLevel > 2 {
		server.ClusterGroup.LogPrintf("INFO", "Trying to rejoin restarted standalone server %s", server.URL)
	}
	server.ClusterGroup.canFlashBack = true
	if server.ClusterGroup.main != nil {
		if server.URL != server.ClusterGroup.main.URL {
			server.ClusterGroup.SetState("WARN0022", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["WARN0022"], server.URL, server.ClusterGroup.main.URL), ErrFrom: "REJOIN"})
			crash := server.ClusterGroup.getCrashFromJoiner(server.URL)
			if crash == nil {
				server.ClusterGroup.SetState("ERR00066", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00066"], server.URL, server.ClusterGroup.main.URL), ErrFrom: "REJOIN"})
				if server.ClusterGroup.oldMain != nil {
					if server.ClusterGroup.oldMain.URL == server.URL {
						server.RejoinMainSST()
						server.ClusterGroup.rejoinCond.Send <- true
						return nil
					}
				}
				if server.ClusterGroup.Conf.Autoseed {
					server.ReseedMainSST()
					server.ClusterGroup.rejoinCond.Send <- true
					return nil
				} else {
					server.ClusterGroup.rejoinCond.Send <- true
					return errors.New("No Autoseed")
				}
			}
			if server.ClusterGroup.Conf.AutorejoinBackupBinlog == true {
				server.backupBinlog(crash)
			}

			err := server.rejoinMainIncremental(crash)
			if err != nil {
				server.ClusterGroup.LogPrintf("ERROR", "Failed to autojoin incremental to main %s", server.URL)
				err := server.RejoinMainSST()
				if err != nil {
					server.ClusterGroup.LogPrintf("ERROR", "State transfer rejoin failed")
				}
			}
			if server.ClusterGroup.Conf.AutorejoinBackupBinlog == true {
				server.saveBinlog(crash)
			}

		}
	} else {
		//no main discovered
		if server.ClusterGroup.lastmain != nil {
			if server.ClusterGroup.lastmain.ServerID == server.ServerID {
				server.ClusterGroup.LogPrintf("INFO", "Rediscovering last seen main: %s", server.URL)
				server.ClusterGroup.main = server
				server.ClusterGroup.lastmain = nil
			} else {
				if server.ClusterGroup.Conf.FailRestartUnsafe == false {
					server.ClusterGroup.LogPrintf("INFO", "Rediscovering last seen main: %s", server.URL)

					server.rejoinMainAsSubordinate()

				}
			}
		} else {
			if server.ClusterGroup.Conf.FailRestartUnsafe == true {
				server.ClusterGroup.LogPrintf("INFO", "Restart Unsafe Picking first non-subordinate as main: %s", server.URL)
				server.ClusterGroup.main = server
			}
		}
		// if consul or internal proxy need to adapt read only route to new subordinates
		server.ClusterGroup.backendStateChangeProxies()
	}
	server.ClusterGroup.rejoinCond.Send <- true
	return nil
}

func (server *ServerMonitor) RejoinPreviousSnapshot() error {
	_, err := server.JobZFSSnapBack()
	return err
}

func (server *ServerMonitor) RejoinMainSST() error {
	if server.ClusterGroup.Conf.AutorejoinMysqldump == true {
		server.ClusterGroup.LogPrintf("INFO", "Rejoin flashback dump restore %s", server.URL)
		err := server.RejoinDirectDump()
		if err != nil {
			server.ClusterGroup.LogPrintf("ERROR", "mysqldump flashback restore failed %s", err)
			return errors.New("Dump from main failed")
		}
	} else if server.ClusterGroup.Conf.AutorejoinLogicalBackup {
		server.JobFlashbackLogicalBackup()
	} else if server.ClusterGroup.Conf.AutorejoinPhysicalBackup {
		server.JobFlashbackPhysicalBackup()
	} else if server.ClusterGroup.Conf.AutorejoinZFSFlashback {
		server.RejoinPreviousSnapshot()
	} else if server.ClusterGroup.Conf.RejoinScript != "" {
		server.ClusterGroup.LogPrintf("INFO", "Calling rejoin flashback script")
		var out []byte
		out, err := exec.Command(server.ClusterGroup.Conf.RejoinScript, misc.Unbracket(server.Host), misc.Unbracket(server.ClusterGroup.main.Host)).CombinedOutput()
		if err != nil {
			server.ClusterGroup.LogPrintf("ERROR", "%s", err)
		}
		server.ClusterGroup.LogPrintf("INFO", "Rejoin script complete %s", string(out))
	} else {
		server.ClusterGroup.LogPrintf("INFO", "No SST rejoin method found")
		return errors.New("No SST rejoin flashback method found")
	}

	return nil
}

func (server *ServerMonitor) ReseedMainSST() error {
	if server.ClusterGroup.Conf.AutorejoinMysqldump == true {
		server.ClusterGroup.LogPrintf("INFO", "Rejoin dump restore %s", server.URL)
		err := server.RejoinDirectDump()
		if err != nil {
			server.ClusterGroup.LogPrintf("ERROR", "mysqldump restore failed %s", err)
			return errors.New("Dump from main failed")
		}
	} else if server.ClusterGroup.Conf.AutorejoinLogicalBackup {
		server.JobReseedLogicalBackup()
	} else if server.ClusterGroup.Conf.AutorejoinPhysicalBackup {
		server.JobReseedPhysicalBackup()
	} else if server.ClusterGroup.Conf.RejoinScript != "" {
		server.ClusterGroup.LogPrintf("INFO", "Calling rejoin script")
		var out []byte
		out, err := exec.Command(server.ClusterGroup.Conf.RejoinScript, misc.Unbracket(server.Host), misc.Unbracket(server.ClusterGroup.main.Host)).CombinedOutput()
		if err != nil {
			server.ClusterGroup.LogPrintf("ERROR", "%s", err)
		}
		server.ClusterGroup.LogPrintf("INFO", "Rejoin script complete %s", string(out))
	} else {
		server.ClusterGroup.LogPrintf("INFO", "No SST reseed method found")
		return errors.New("No SST reseed method found")
	}

	return nil
}

func (server *ServerMonitor) rejoinMainSync(crash *Crash) error {
	if server.HasGTIDReplication() {
		server.ClusterGroup.LogPrintf("INFO", "Found same or lower GTID %s and new elected main was %s", server.CurrentGtid.Sprint(), crash.FailoverIOGtid.Sprint())
	} else {
		server.ClusterGroup.LogPrintf("INFO", "Found same or lower sequence %s , %s", server.BinaryLogFile, server.BinaryLogPos)
	}
	var err error
	realmain := server.ClusterGroup.main
	if server.ClusterGroup.Conf.MxsBinlogOn || server.ClusterGroup.Conf.MultiTierSubordinate {
		realmain = server.ClusterGroup.GetRelayServer()
	}
	if server.HasGTIDReplication() || (realmain.MxsHaveGtid && realmain.IsMaxscale) {
		logs, err := server.SetReplicationGTIDCurrentPosFromServer(realmain)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed in GTID rejoin old main in sync %s, %s", server.URL, err)
		if err != nil {
			return err
		}
	} else if server.ClusterGroup.Conf.MxsBinlogOn {
		logs, err := dbhelper.ChangeMain(server.Conn, dbhelper.ChangeMainOpt{
			Host:      realmain.Host,
			Port:      realmain.Port,
			User:      server.ClusterGroup.rplUser,
			Password:  server.ClusterGroup.rplPass,
			Retry:     strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatRetry),
			Heartbeat: strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatTime),
			Mode:      "MXS",
			Logfile:   crash.FailoverMainLogFile,
			Logpos:    crash.FailoverMainLogPos,
			SSL:       server.ClusterGroup.Conf.ReplicationSSL,
		}, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Change main positional failed in Rejoin old Main in sync to maxscale %s", err)
		if err != nil {
			return err
		}
	} else {
		// not maxscale the new main coordonate are in crash
		server.ClusterGroup.LogPrintf("INFO", "Change main to positional in Rejoin old Main")
		logs, err := dbhelper.ChangeMain(server.Conn, dbhelper.ChangeMainOpt{
			Host:        realmain.Host,
			Port:        realmain.Port,
			User:        server.ClusterGroup.rplUser,
			Password:    server.ClusterGroup.rplPass,
			Retry:       strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatRetry),
			Heartbeat:   strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatTime),
			Mode:        "POSITIONAL",
			Logfile:     crash.NewMainLogFile,
			Logpos:      crash.NewMainLogPos,
			SSL:         server.ClusterGroup.Conf.ReplicationSSL,
			Channel:     server.ClusterGroup.Conf.MainConn,
			IsDelayed:   server.IsDelayed,
			Delay:       strconv.Itoa(server.ClusterGroup.Conf.HostsDelayedTime),
			PostgressDB: server.PostgressDB,
		}, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Change main positional failed in Rejoin old Main in sync %s", err)
		if err != nil {
			return err
		}
	}

	server.StartSubordinate()
	return err
}

func (server *ServerMonitor) rejoinMainFlashBack(crash *Crash) error {
	realmain := server.ClusterGroup.main
	if server.ClusterGroup.Conf.MxsBinlogOn || server.ClusterGroup.Conf.MultiTierSubordinate {
		realmain = server.ClusterGroup.GetRelayServer()
	}

	if _, err := os.Stat(server.ClusterGroup.GetMysqlBinlogPath()); os.IsNotExist(err) {
		server.ClusterGroup.LogPrintf("ERROR", "File does not exist %s", server.ClusterGroup.GetMysqlBinlogPath())
		return err
	}
	if _, err := os.Stat(server.ClusterGroup.GetMysqlclientPath()); os.IsNotExist(err) {
		server.ClusterGroup.LogPrintf("ERROR", "File does not exist %s", server.ClusterGroup.GetMysqlclientPath())
		return err
	}

	binlogCmd := exec.Command(server.ClusterGroup.GetMysqlBinlogPath(), "--flashback", "--to-last-log", server.ClusterGroup.Conf.WorkingDir+"/"+server.ClusterGroup.Name+"-server"+strconv.FormatUint(uint64(server.ServerID), 10)+"-"+crash.FailoverMainLogFile)
	clientCmd := exec.Command(server.ClusterGroup.GetMysqlclientPath(), "--host="+misc.Unbracket(server.Host), "--port="+server.Port, "--user="+server.ClusterGroup.dbUser, "--password="+server.ClusterGroup.dbPass)
	server.ClusterGroup.LogPrintf("INFO", "FlashBack: %s %s", server.ClusterGroup.GetMysqlBinlogPath(), strings.Replace(strings.Join(binlogCmd.Args, " "), server.ClusterGroup.rplPass, "XXXX", -1))
	var err error
	clientCmd.Stdin, err = binlogCmd.StdoutPipe()
	if err != nil {
		server.ClusterGroup.LogPrintf("ERROR", "Error opening pipe: %s", err)
		return err
	}
	if err := binlogCmd.Start(); err != nil {
		server.ClusterGroup.LogPrintf("ERROR", "Failed mysqlbinlog command: %s at %s", err, strings.Replace(binlogCmd.Path, server.ClusterGroup.rplPass, "XXXX", -1))
		return err
	}
	if err := clientCmd.Run(); err != nil {
		server.ClusterGroup.LogPrintf("ERROR", "Error starting client: %s at %s", err, strings.Replace(clientCmd.Path, server.ClusterGroup.rplPass, "XXXX", -1))
		return err
	}
	logs, err := dbhelper.SetGTIDSubordinatePos(server.Conn, crash.FailoverIOGtid.Sprint())
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlInfo, "SET GLOBAL gtid_subordinate_pos = \"%s\"", crash.FailoverIOGtid.Sprint())
	if err != nil {
		return err
	}
	var err2 error
	if server.MxsHaveGtid || server.IsMaxscale == false {
		logs, err2 = server.SetReplicationGTIDSubordinatePosFromServer(realmain)
	} else {
		logs, err2 = server.SetReplicationFromMaxsaleServer(realmain)
	}
	server.ClusterGroup.LogSQL(logs, err2, server.URL, "Rejoin", LvlInfo, "Failed SetReplicationGTIDSubordinatePosFromServer on %s: %s", server.URL, err2)
	if err2 != nil {
		return err2
	}
	logs, err = server.StartSubordinate()
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlInfo, "Failed stop subordinate on %s: %s", server.URL, err)

	return nil
}

func (server *ServerMonitor) RejoinDirectDump() error {
	var err3 error

	realmain := server.ClusterGroup.main
	if server.ClusterGroup.Conf.MxsBinlogOn || server.ClusterGroup.Conf.MultiTierSubordinate {
		realmain = server.ClusterGroup.GetRelayServer()
	}
	// done change main just to set the host and port before dump
	if server.MxsHaveGtid || server.IsMaxscale == false {
		logs, err3 := server.SetReplicationGTIDSubordinatePosFromServer(realmain)
		server.ClusterGroup.LogSQL(logs, err3, server.URL, "Rejoin", LvlInfo, "Failed SetReplicationGTIDSubordinatePosFromServer on %s: %s", server.URL, err3)

	} else {
		logs, err3 := dbhelper.ChangeMain(server.Conn, dbhelper.ChangeMainOpt{
			Host:      realmain.Host,
			Port:      realmain.Port,
			User:      server.ClusterGroup.rplUser,
			Password:  server.ClusterGroup.rplPass,
			Retry:     strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatRetry),
			Heartbeat: strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatTime),
			Mode:      "MXS",
			Logfile:   realmain.FailoverMainLogFile,
			Logpos:    realmain.FailoverMainLogPos,
			SSL:       server.ClusterGroup.Conf.ReplicationSSL,
		}, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err3, server.URL, "Rejoin", LvlErr, "Failed change main maxscale on %s: %s", server.URL, err3)
	}
	if err3 != nil {
		return err3
	}
	// dump here
	backupserver := server.ClusterGroup.GetBackupServer()
	if backupserver == nil {
		go server.ClusterGroup.JobRejoinMysqldumpFromSource(server.ClusterGroup.main, server)
	} else {
		go server.ClusterGroup.JobRejoinMysqldumpFromSource(backupserver, server)
	}
	return nil
}

func (server *ServerMonitor) rejoinMainIncremental(crash *Crash) error {
	server.ClusterGroup.LogPrintf("INFO", "Rejoin main incremental %s", server.URL)
	server.ClusterGroup.LogPrintf("INFO", "Crash info %s", crash)
	server.Refresh()
	if server.ClusterGroup.Conf.ReadOnly && !server.ClusterGroup.IsInIgnoredReadonly(server) {
		logs, err := server.SetReadOnly()
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed to set read only on server %s, %s ", server.URL, err)
	}

	if crash.FailoverIOGtid != nil {
		server.ClusterGroup.LogPrintf("INFO", "Rejoined GTID sequence %d", server.CurrentGtid.GetSeqServerIdNos(uint64(server.ServerID)))
		server.ClusterGroup.LogPrintf("INFO", "Crash Saved GTID sequence %d for main id %d", crash.FailoverIOGtid.GetSeqServerIdNos(uint64(server.ServerID)), uint64(server.ServerID))
	}
	if server.isReplicationAheadOfMainElection(crash) == false || server.ClusterGroup.Conf.MxsBinlogOn {
		server.rejoinMainSync(crash)
		return nil
	} else {
		// don't try flashback on old style replication that are ahead jump to SST
		if server.HasGTIDReplication() == false {
			return errors.New("Incremental failed")
		}
	}
	if crash.FailoverIOGtid != nil {
		// server.ClusterGroup.main.FailoverIOGtid.GetSeqServerIdNos(uint64(server.ServerID)) == 0
		// lookup in crash recorded is the current main
		if crash.FailoverIOGtid.GetSeqServerIdNos(uint64(server.ClusterGroup.main.ServerID)) == 0 {
			server.ClusterGroup.LogPrintf("INFO", "Cascading failover, consider we cannot flashback")
			server.ClusterGroup.canFlashBack = false
		} else {
			server.ClusterGroup.LogPrintf("INFO", "Found server ID in rejoining ID %s and crash FailoverIOGtid %s Main %s", server.ServerID, crash.FailoverIOGtid.Sprint(), server.ClusterGroup.main.URL)
		}
	} else {
		server.ClusterGroup.LogPrintf("INFO", "Old server GTID for flashback not found")
	}
	if crash.FailoverIOGtid != nil && server.ClusterGroup.canFlashBack == true && server.ClusterGroup.Conf.AutorejoinFlashback == true && server.ClusterGroup.Conf.AutorejoinBackupBinlog == true {
		err := server.rejoinMainFlashBack(crash)
		if err == nil {
			return nil
		}
		server.ClusterGroup.LogPrintf("ERROR", "Flashback rejoin failed: %s", err)
		return errors.New("Flashback failed")
	} else {
		server.ClusterGroup.LogPrintf("INFO", "No flashback rejoin can flashback %t, autorejoin-flashback %t autorejoin-backup-binlog %t", server.ClusterGroup.canFlashBack, server.ClusterGroup.Conf.AutorejoinFlashback, server.ClusterGroup.Conf.AutorejoinBackupBinlog)
		return errors.New("Flashback disabled")
	}

}

func (server *ServerMonitor) rejoinMainAsSubordinate() error {
	realmain := server.ClusterGroup.lastmain
	server.ClusterGroup.LogPrintf("INFO", "Rejoining old main server %s to saved main %s", server.URL, realmain.URL)
	logs, err := server.SetReadOnly()
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed to set read only on server %s, %s ", server.URL, err)
	if err == nil {
		logs, err = server.SetReplicationGTIDCurrentPosFromServer(realmain)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed to autojoin indirect main server %s, stopping subordinate as a precaution %s ", server.URL, err)
		if err == nil {
			logs, err = server.StartSubordinate()
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed to stop subordinate on erver %s, %s ", server.URL, err)
		} else {

			return err
		}
	} else {
		server.ClusterGroup.LogPrintf("ERROR", "Rejoin main as subordinate can't set read only %s", err)
		return err
	}
	return nil
}

func (server *ServerMonitor) rejoinSubordinate(ss dbhelper.SubordinateStatus) error {
	// Test if subordinate not connected to current main
	if server.ClusterGroup.GetTopology() == topoMultiMainRing || server.ClusterGroup.GetTopology() == topoMultiMainWsrep {
		if server.ClusterGroup.GetTopology() == topoMultiMainRing {
			server.RejoinLoop()
			server.ClusterGroup.rejoinCond.Send <- true
			return nil
		}
	}
	mycurrentmain, _ := server.ClusterGroup.GetMainFromReplication(server)
	if mycurrentmain == nil {
		server.ClusterGroup.LogPrintf(LvlErr, "No main found from replication")
		server.ClusterGroup.rejoinCond.Send <- true
		return errors.New("No main found from replication")
	}
	if server.ClusterGroup.main != nil && mycurrentmain != nil {

		server.ClusterGroup.SetState("ERR00067", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00067"], server.URL, server.PrevState, ss.SubordinateIORunning.String, server.ClusterGroup.main.URL), ErrFrom: "REJOIN"})

		if mycurrentmain.IsMaxscale == false && server.ClusterGroup.Conf.MultiTierSubordinate == false && server.ClusterGroup.Conf.ReplicationNoRelay {

			if server.HasGTIDReplication() {
				crash := server.ClusterGroup.getCrashFromMain(server.ClusterGroup.main.URL)
				if crash == nil {
					server.ClusterGroup.SetState("ERR00065", state.State{ErrType: "ERROR", ErrDesc: fmt.Sprintf(clusterError["ERR00065"], server.URL, server.ClusterGroup.main.URL), ErrFrom: "REJOIN"})
					server.ClusterGroup.rejoinCond.Send <- true
					return errors.New("No Crash info on current main")
				}
				server.ClusterGroup.LogPrintf("INFO", "Crash info on current main %s", crash)
				server.ClusterGroup.LogPrintf("INFO", "Found subordinate to rejoin %s subordinate was previously in state %s replication io thread  %s, pointing currently to %s", server.URL, server.PrevState, ss.SubordinateIORunning, server.ClusterGroup.main.URL)

				realmain := server.ClusterGroup.main
				// A SLAVE IS ALWAY BEHIND MASTER
				//		subordinate_gtid := server.CurrentGtid.GetSeqServerIdNos(uint64(server.GetReplicationServerID()))
				//		main_gtid := crash.FailoverIOGtid.GetSeqServerIdNos(uint64(server.GetReplicationServerID()))
				//	if subordinate_gtid < main_gtid {
				server.ClusterGroup.LogPrintf("INFO", "Rejoining subordinate via GTID")
				logs, err := server.StopSubordinate()
				server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed to stop subordinate server %s, stopping subordinate as a precaution %s", server.URL, err)
				if err == nil {
					logs, err := server.SetReplicationGTIDSubordinatePosFromServer(realmain)
					server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed to autojoin indirect subordinate server %s, stopping subordinate as a precaution %s", server.URL, err)
					if err == nil {
						logs, err := server.StartSubordinate()
						server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed to start  subordinate server %s, stopping subordinate as a precaution %s", server.URL, err)
					}
				}
			} else {
				if mycurrentmain.State != stateFailed && mycurrentmain.IsRelay {
					// No GTID compatible solution stop relay main wait apply relay and move to real main
					logs, err := mycurrentmain.StopSubordinate()
					server.ClusterGroup.LogSQL(logs, err, mycurrentmain.URL, "Rejoin", LvlErr, "Failed to stop subordinate on relay server  %s: %s", mycurrentmain.URL, err)
					if err == nil {
						logs, err2 := dbhelper.MainPosWait(server.Conn, mycurrentmain.BinaryLogFile, mycurrentmain.BinaryLogPos, 3600)
						server.ClusterGroup.LogSQL(logs, err2, server.URL, "Rejoin", LvlErr, "Failed positional rejoin wait pos %s %s", server.URL, err2)
						if err2 == nil {
							myparentss, _ := mycurrentmain.GetSubordinateStatus(mycurrentmain.ReplicationSourceName)

							logs, err := server.StopSubordinate()
							server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed to stop subordinate on server %s: %s", server.URL, err)
							server.ClusterGroup.LogPrintf("INFO", "Doing Positional switch of subordinate %s", server.URL)
							logs, changeMainErr := dbhelper.ChangeMain(server.Conn, dbhelper.ChangeMainOpt{
								Host:        server.ClusterGroup.main.Host,
								Port:        server.ClusterGroup.main.Port,
								User:        server.ClusterGroup.rplUser,
								Password:    server.ClusterGroup.rplPass,
								Logfile:     myparentss.MainLogFile.String,
								Logpos:      myparentss.ReadMainLogPos.String,
								Retry:       strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatRetry),
								Heartbeat:   strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatTime),
								Channel:     server.ClusterGroup.Conf.MainConn,
								IsDelayed:   server.IsDelayed,
								Delay:       strconv.Itoa(server.ClusterGroup.Conf.HostsDelayedTime),
								SSL:         server.ClusterGroup.Conf.ReplicationSSL,
								PostgressDB: server.PostgressDB,
							}, server.DBVersion)

							server.ClusterGroup.LogSQL(logs, changeMainErr, server.URL, "Rejoin", LvlErr, "Rejoin Failed doing Positional switch of subordinate %s: %s", server.URL, changeMainErr)

						}
						logs, err = server.StartSubordinate()
						server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed to start subordinate on %s: %s", server.URL, err)

					}
					mycurrentmain.StartSubordinate()
					server.ClusterGroup.LogSQL(logs, err, mycurrentmain.URL, "Rejoin", LvlErr, "Failed to start subordinate on %s: %s", mycurrentmain.URL, err)

					if server.IsMaintenance {
						server.SwitchMaintenance()
					}
					// if consul or internal proxy need to adapt read only route to new subordinates
					server.ClusterGroup.backendStateChangeProxies()

				} else {
					//Adding state waiting for old main to rejoin in positional mode
					// this state prevent crash info to be removed
					server.ClusterGroup.sme.AddState("ERR00049", state.State{ErrType: "ERRRO", ErrDesc: fmt.Sprintf(clusterError["ERR00049"]), ErrFrom: "TOPO"})
				}
			}
		}
	}
	// In case of state change, reintroduce the server in the subordinate list
	if server.PrevState == stateFailed || server.PrevState == stateUnconn || server.PrevState == stateSuspect {
		server.ClusterGroup.LogPrintf(LvlInfo, "Set stateSubordinate from rejoin subordinate %s", server.URL)
		server.State = stateSubordinate
		server.FailCount = 0
		if server.PrevState != stateSuspect {
			server.ClusterGroup.subordinates = append(server.ClusterGroup.subordinates, server)
		}
		if server.ClusterGroup.Conf.ReadOnly {
			logs, err := dbhelper.SetReadOnly(server.Conn, true)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlErr, "Failed to set read only on server %s, %s ", server.URL, err)
			if err != nil {
				server.ClusterGroup.rejoinCond.Send <- true
				return err
			}
		}
	}
	server.ClusterGroup.rejoinCond.Send <- true
	return nil
}

func (server *ServerMonitor) isReplicationAheadOfMainElection(crash *Crash) bool {

	if server.UsedGtidAtElection(crash) {

		// CurrentGtid fetch from show global variables GTID_CURRENT_POS
		// FailoverIOGtid is fetch at failover from show subordinate status of the new main
		// If server-id can't be found in FailoverIOGtid can state cascading main failover
		if crash.FailoverIOGtid.GetSeqServerIdNos(uint64(server.ServerID)) == 0 {
			server.ClusterGroup.LogPrintf("INFO", "Cascading failover, found empty GTID, forcing full state transfer")
			return true
		}
		if server.CurrentGtid.GetSeqServerIdNos(uint64(server.ServerID)) > crash.FailoverIOGtid.GetSeqServerIdNos(uint64(server.ServerID)) {
			server.ClusterGroup.LogPrintf("INFO", "Rejoining node seq %d, main seq %d", server.CurrentGtid.GetSeqServerIdNos(uint64(server.ServerID)), crash.FailoverIOGtid.GetSeqServerIdNos(uint64(server.ServerID)))
			return true
		}
		return false
	} else {
		/*ss, errss := server.GetSubordinateStatus(server.ReplicationSourceName)
		if errss != nil {
		 return	false
		}*/
		valid, logs, err := dbhelper.HaveExtraEvents(server.Conn, crash.FailoverMainLogFile, crash.FailoverMainLogPos)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Rejoin", LvlDbg, "Failed to  get extra bin log events server %s, %s ", server.URL, err)
		if err != nil {
			return false
		}
		if valid {
			server.ClusterGroup.LogPrintf("INFO", "No extra events after  file %s, pos %d is equal ", crash.FailoverMainLogFile, crash.FailoverMainLogPos)
			return true
		}
		return false
	}
}

func (server *ServerMonitor) deletefiles(path string, f os.FileInfo, err error) (e error) {

	// check each file if starts with the word "dumb_"
	if strings.HasPrefix(f.Name(), server.ClusterGroup.Name+"-server"+strconv.FormatUint(uint64(server.ServerID), 10)+"-") {
		os.Remove(path)
	}
	return
}

func (server *ServerMonitor) saveBinlog(crash *Crash) error {
	t := time.Now()
	backupdir := server.ClusterGroup.Conf.WorkingDir + "/" + server.ClusterGroup.Name + "/crash-bin-" + t.Format("20060102150405")
	server.ClusterGroup.LogPrintf("INFO", "Rejoin old Main %s , backing up lost event to %s", crash.URL, backupdir)
	os.Mkdir(backupdir, 0777)
	os.Rename(server.ClusterGroup.Conf.WorkingDir+"/"+server.ClusterGroup.Name+"-server"+strconv.FormatUint(uint64(server.ServerID), 10)+"-"+crash.FailoverMainLogFile, backupdir+"/"+server.ClusterGroup.Name+"-server"+strconv.FormatUint(uint64(server.ServerID), 10)+"-"+crash.FailoverMainLogFile)
	return nil

}
func (server *ServerMonitor) backupBinlog(crash *Crash) error {

	if _, err := os.Stat(server.ClusterGroup.GetMysqlBinlogPath()); os.IsNotExist(err) {
		server.ClusterGroup.LogPrintf("ERROR", "mysqlbinlog does not exist %s check binary path", server.ClusterGroup.GetMysqlBinlogPath())
		return err
	}
	if _, err := os.Stat(server.ClusterGroup.Conf.WorkingDir); os.IsNotExist(err) {
		server.ClusterGroup.LogPrintf("ERROR", "WorkingDir does not exist %s check param working-directory", server.ClusterGroup.Conf.WorkingDir)
		return err
	}
	var cmdrun *exec.Cmd
	server.ClusterGroup.LogPrintf("INFO", "Backup ahead binlog events of previously failed server %s", server.URL)
	filepath.Walk(server.ClusterGroup.Conf.WorkingDir+"/", server.deletefiles)

	cmdrun = exec.Command(server.ClusterGroup.GetMysqlBinlogPath(), "--read-from-remote-server", "--raw", "--stop-never-subordinate-server-id=10000", "--user="+server.ClusterGroup.rplUser, "--password="+server.ClusterGroup.rplPass, "--host="+misc.Unbracket(server.Host), "--port="+server.Port, "--result-file="+server.ClusterGroup.Conf.WorkingDir+"/"+server.ClusterGroup.Name+"-server"+strconv.FormatUint(uint64(server.ServerID), 10)+"-", "--start-position="+crash.FailoverMainLogPos, crash.FailoverMainLogFile)
	server.ClusterGroup.LogPrintf("INFO", "Backup %s %s", server.ClusterGroup.GetMysqlBinlogPath(), strings.Replace(strings.Join(cmdrun.Args, " "), server.ClusterGroup.rplPass, "XXXX", -1))

	var outrun bytes.Buffer
	cmdrun.Stdout = &outrun
	var outrunerr bytes.Buffer
	cmdrun.Stderr = &outrunerr

	cmdrunErr := cmdrun.Run()
	if cmdrunErr != nil {
		server.ClusterGroup.LogPrintf("ERROR", "Failed to backup binlogs of %s,%s", server.URL, cmdrunErr.Error())
		server.ClusterGroup.LogPrintf("ERROR", "%s %s", server.ClusterGroup.GetMysqlBinlogPath(), cmdrun.Args)
		server.ClusterGroup.LogPrint(cmdrun.Stderr)
		server.ClusterGroup.LogPrint(cmdrun.Stdout)
		server.ClusterGroup.canFlashBack = false
		return cmdrunErr
	}
	return nil
}

func (cluster *Cluster) RejoinClone(source *ServerMonitor, dest *ServerMonitor) error {
	cluster.LogPrintf(LvlInfo, "Rejoining via main clone ")
	if dest.DBVersion.IsMySQL() && dest.DBVersion.Major >= 8 {
		if !dest.HasInstallPlugin("CLONE") {
			cluster.LogPrintf(LvlInfo, "Installing Clone plugin")
			dest.InstallPlugin("CLONE")
		}
		dest.ExecQueryNoBinLog("set global clone_valid_donor_list = '" + source.Host + ":" + source.Port + "'")
		dest.ExecQueryNoBinLog("CLONE INSTANCE FROM " + dest.User + "@" + source.Host + ":" + source.Port + " identified by '" + dest.Pass + "'")
		cluster.LogPrintf(LvlInfo, "Start subordinate after dump")
		dest.SetReplicationGTIDSubordinatePosFromServer(source)
		dest.StartSubordinate()
	} else {
		return errors.New("Version does not support cloning Main")
	}
	return nil
}

func (cluster *Cluster) RejoinFixRelay(subordinate *ServerMonitor, relay *ServerMonitor) error {
	if cluster.GetTopology() == topoMultiMainRing || cluster.GetTopology() == topoMultiMainWsrep {
		return nil
	}
	cluster.sme.AddState("ERR00045", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00045"]), ErrFrom: "TOPO"})

	if subordinate.GetReplicationDelay() > cluster.Conf.FailMaxDelay {
		cluster.sme.AddState("ERR00046", state.State{ErrType: "WARNING", ErrDesc: fmt.Sprintf(clusterError["ERR00046"]), ErrFrom: "TOPO"})
		return nil
	} else {
		ss, err := subordinate.GetSubordinateStatus(subordinate.ReplicationSourceName)
		if err == nil {
			subordinate.rejoinSubordinate(*ss)
		}
	}

	return nil
}

// UseGtid  check is replication use gtid
func (server *ServerMonitor) UsedGtidAtElection(crash *Crash) bool {
	ss, errss := server.GetSubordinateStatus(server.ReplicationSourceName)
	if errss != nil {
		return false
	}

	server.ClusterGroup.LogPrintf(LvlDbg, "Rejoin Server use GTID %s", ss.UsingGtid.String)

	// An old main  main do no have replication
	if crash.FailoverIOGtid == nil {
		server.ClusterGroup.LogPrintf(LvlDbg, "Rejoin server cannot find a saved main election GTID")
		return false
	}
	if len(crash.FailoverIOGtid.GetSeqNos()) > 0 {
		return true
	} else {
		return false
	}
}
