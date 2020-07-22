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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc64"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"

	"github.com/go-sql-driver/mysql"
	"github.com/hpcloud/tail"
	"github.com/jmoiron/sqlx"
	"github.com/signal18/replication-manager/utils/dbhelper"
	"github.com/signal18/replication-manager/utils/gtid"
	"github.com/signal18/replication-manager/utils/misc"
	"github.com/signal18/replication-manager/utils/s18log"
	"github.com/signal18/replication-manager/utils/state"
)

// ServerMonitor defines a server to monitor.
type ServerMonitor struct {
	Id                          string                       `json:"id"` //Unique name given by cluster & crc64(URL) used by test to provision
	Name                        string                       `json:"name"`
	Domain                      string                       `json:"domain"`
	ServiceName                 string                       `json:"serviceName"`
	Conn                        *sqlx.DB                     `json:"-"`
	User                        string                       `json:"user"`
	Pass                        string                       `json:"-"`
	URL                         string                       `json:"url"`
	DSN                         string                       `json:"dsn"`
	Host                        string                       `json:"host"`
	Port                        string                       `json:"port"`
	TunnelPort                  string                       `json:"tunnelPort"`
	IP                          string                       `json:"ip"`
	Strict                      string                       `json:"strict"`
	ServerID                    uint64                       `json:"serverId"`
	GTIDBinlogPos               *gtid.List                   `json:"gtidBinlogPos"`
	CurrentGtid                 *gtid.List                   `json:"currentGtid"`
	SubordinateGtid                   *gtid.List                   `json:"subordinateGtid"`
	IOGtid                      *gtid.List                   `json:"ioGtid"`
	FailoverIOGtid              *gtid.List                   `json:"failoverIoGtid"`
	GTIDExecuted                string                       `json:"gtidExecuted"`
	ReadOnly                    string                       `json:"readOnly"`
	State                       string                       `json:"state"`
	PrevState                   string                       `json:"prevState"`
	FailCount                   int                          `json:"failCount"`
	FailSuspectHeartbeat        int64                        `json:"failSuspectHeartbeat"`
	ClusterGroup                *Cluster                     `json:"-"` //avoid recusive json
	BinaryLogFile               string                       `json:"binaryLogFile"`
	BinaryLogFilePrevious       string                       `json:"binaryLogFilePrevious"`
	BinaryLogPos                string                       `json:"binaryLogPos"`
	FailoverMainLogFile       string                       `json:"failoverMainLogFile"`
	FailoverMainLogPos        string                       `json:"failoverMainLogPos"`
	FailoverSemiSyncSubordinateStatus bool                         `json:"failoverSemiSyncSubordinateStatus"`
	Process                     *os.Process                  `json:"process"`
	SemiSyncMainStatus        bool                         `json:"semiSyncMainStatus"`
	SemiSyncSubordinateStatus         bool                         `json:"semiSyncSubordinateStatus"`
	RplMainStatus             bool                         `json:"rplMainStatus"`
	HaveEventScheduler          bool                         `json:"eventScheduler"`
	HaveSemiSync                bool                         `json:"haveSemiSync"`
	HaveInnodbTrxCommit         bool                         `json:"haveInnodbTrxCommit"`
	HaveChecksum                bool                         `json:"haveInnodbChecksum"`
	HaveLogGeneral              bool                         `json:"haveLogGeneral"`
	HaveBinlog                  bool                         `json:"haveBinlog"`
	HaveBinlogSync              bool                         `json:"haveBinLogSync"`
	HaveBinlogRow               bool                         `json:"haveBinlogRow"`
	HaveBinlogAnnotate          bool                         `json:"haveBinlogAnnotate"`
	HaveBinlogSlowqueries       bool                         `json:"haveBinlogSlowqueries"`
	HaveBinlogCompress          bool                         `json:"haveBinlogCompress"`
	HaveBinlogSubordinateUpdates      bool                         `json:"HaveBinlogSubordinateUpdates"`
	HaveGtidStrictMode          bool                         `json:"haveGtidStrictMode"`
	HaveMySQLGTID               bool                         `json:"haveMysqlGtid"`
	HaveMariaDBGTID             bool                         `json:"haveMariadbGtid"`
	HaveSlowQueryLog            bool                         `json:"haveSlowQueryLog"`
	HavePFSSlowQueryLog         bool                         `json:"havePFSSlowQueryLog"`
	HaveMetaDataLocksLog        bool                         `json:"haveMetaDataLocksLog"`
	HaveQueryResponseTimeLog    bool                         `json:"haveQueryResponseTimeLog"`
	HaveDiskMonitor             bool                         `json:"haveDiskMonitor"`
	HaveSQLErrorLog             bool                         `json:"haveSQLErrorLog"`
	HavePFS                     bool                         `json:"havePFS"`
	HaveWsrep                   bool                         `json:"haveWsrep"`
	HaveReadOnly                bool                         `json:"haveReadOnly"`
	IsWsrepSync                 bool                         `json:"isWsrepSync"`
	IsWsrepDonor                bool                         `json:"isWsrepDonor"`
	IsWsrepPrimary              bool                         `json:"isWsrepPrimary"`
	IsMaxscale                  bool                         `json:"isMaxscale"`
	IsRelay                     bool                         `json:"isRelay"`
	IsSubordinate                     bool                         `json:"isSubordinate"`
	IsVirtualMain             bool                         `json:"isVirtualMain"`
	IsMaintenance               bool                         `json:"isMaintenance"`
	IsCompute                   bool                         `json:"isCompute"` //Used to idenfied spider compute nide
	IsDelayed                   bool                         `json:"isDelayed"`
	Ignored                     bool                         `json:"ignored"`
	Prefered                    bool                         `json:"prefered"`
	PreferedBackup              bool                         `json:"preferedBackup"`
	InCaptureMode               bool                         `json:"inCaptureMode"`
	LongQueryTimeSaved          string                       `json:"longQueryTimeSaved"`
	LongQueryTime               string                       `json:"longQueryTime"`
	LogOutput                   string                       `json:"logOutput"`
	SlowQueryLog                string                       `json:"slowQueryLog"`
	SlowQueryCapture            bool                         `json:"slowQueryCapture"`
	BinlogDumpThreads           int                          `json:"binlogDumpThreads"`
	MxsVersion                  int                          `json:"maxscaleVersion"`
	MxsHaveGtid                 bool                         `json:"maxscaleHaveGtid"`
	MxsServerName               string                       `json:"maxscaleServerName"` //Unique server Name in maxscale conf
	MxsServerStatus             string                       `json:"maxscaleServerStatus"`
	ProxysqlHostgroup           string                       `json:"proxysqlHostgroup"`
	RelayLogSize                uint64                       `json:"relayLogSize"`
	Replications                []dbhelper.SubordinateStatus       `json:"replications"`
	LastSeenReplications        []dbhelper.SubordinateStatus       `json:"lastSeenReplications"`
	MainStatus                dbhelper.MainStatus        `json:"mainStatus"`
	SubordinateStatus                 *dbhelper.SubordinateStatus        `json:"-"`
	ReplicationSourceName       string                       `json:"replicationSourceName"`
	DBVersion                   *dbhelper.MySQLVersion       `json:"dbVersion"`
	Version                     int                          `json:"-"`
	QPS                         int64                        `json:"qps"`
	ReplicationHealth           string                       `json:"replicationHealth"`
	EventStatus                 []dbhelper.Event             `json:"eventStatus"`
	FullProcessList             []dbhelper.Processlist       `json:"-"`
	Variables                   map[string]string            `json:"-"`
	EngineInnoDB                map[string]string            `json:"engineInnodb"`
	ErrorLog                    s18log.HttpLog               `json:"errorLog"`
	SlowLog                     s18log.SlowLog               `json:"-"`
	Status                      map[string]string            `json:"-"`
	PrevStatus                  map[string]string            `json:"-"`
	PFSQueries                  map[string]dbhelper.PFSQuery `json:"-"` //PFS queries
	SlowPFSQueries              map[string]dbhelper.PFSQuery `json:"-"` //PFS queries from slow
	DictTables                  map[string]dbhelper.Table    `json:"-"`
	Tables                      []dbhelper.Table             `json:"-"`
	Disks                       []dbhelper.Disk              `json:"-"`
	Plugins                     map[string]dbhelper.Plugin   `json:"-"`
	Users                       map[string]dbhelper.Grant    `json:"-"`
	MetaDataLocks               []dbhelper.MetaDataLock      `json:"-"`
	ErrorLogTailer              *tail.Tail                   `json:"-"`
	SlowLogTailer               *tail.Tail                   `json:"-"`
	MonitorTime                 int64                        `json:"-"`
	PrevMonitorTime             int64                        `json:"-"`
	maxConn                     string                       `json:"maxConn"` // used to back max connection for failover
	Datadir                     string                       `json:"-"`
	SlapOSDatadir               string                       `json:"slaposDatadir"`
	PostgressDB                 string                       `json:"postgressDB"`
	CrcTable                    *crc64.Table                 `json:"-"`
	TLSConfigUsed               string                       `json:"tlsConfigUsed"` //used to track TLS config during key rotation
	SSTPort                     string                       `json:"sstPort"`       //used to send data to dbjobs
	BinaryLogFiles              map[string]uint              `json:"binaryLogFiles"`
}

type serverList []*ServerMonitor

const (
	stateFailed       string = "Failed"
	stateMain       string = "Main"
	stateSubordinate        string = "Subordinate"
	stateSubordinateErr     string = "SubordinateErr"
	stateSubordinateLate    string = "SubordinateLate"
	stateMaintenance  string = "Maintenance"
	stateUnconn       string = "StandAlone"
	stateErrorAuth    string = "ErrorAuth"
	stateSuspect      string = "Suspect"
	stateShard        string = "Shard"
	stateProv         string = "Provision"
	stateMainAlone  string = "MainAlone"
	stateRelay        string = "Relay"
	stateRelayErr     string = "RelayErr"
	stateRelayLate    string = "RelayLate"
	stateWsrep        string = "Wsrep"
	stateWsrepDonor   string = "WsrepDonor"
	stateWsrepLate    string = "WsrepUnsync"
	stateProxyRunning string = "ProxyRunning"
	stateProxyDesync  string = "ProxyDesync"
)

const (
	ConstTLSNoConfig      string = ""
	ConstTLSOldConfig     string = "&tls=tlsconfigold"
	ConstTLSCurrentConfig string = "&tls=tlsconfig"
)

/* Initializes a server object compute if spider node*/
func (cluster *Cluster) newServerMonitor(url string, user string, pass string, compute bool, domain string) (*ServerMonitor, error) {
	var err error
	server := new(ServerMonitor)
	server.QPS = 0
	server.IsCompute = compute
	server.Domain = domain
	server.TLSConfigUsed = ConstTLSCurrentConfig
	server.CrcTable = crc64.MakeTable(crc64.ECMA)
	server.ClusterGroup = cluster
	server.DBVersion = dbhelper.NewMySQLVersion("Unknowed-0.0.0", "")
	server.CheckVersion()
	server.Name, server.Port, server.PostgressDB = misc.SplitHostPortDB(url)
	server.ClusterGroup = cluster
	server.ServiceName = cluster.Name + "/svc/" + server.Name
	if cluster.Conf.ProvNetCNI {
		/*	if server.IsCompute && cluster.Conf.ClusterHead != "" {
				url = server.Name + "." + cluster.Conf.ClusterHead + ".svc." + server.ClusterGroup.Conf.ProvNetCNICluster + ":3306"
			} else {
				url = server.Name + "." + cluster.Name + ".svc." + server.ClusterGroup.Conf.ProvNetCNICluster + ":3306"
			}*/
		url = server.Name + server.Domain + ":3306"
	}
	server.Id = "db" + strconv.FormatUint(crc64.Checksum([]byte(cluster.Name+server.Name+server.Port), crcTable), 10)
	var sid uint64
	sid, err = strconv.ParseUint(strconv.FormatUint(crc64.Checksum([]byte(server.Name+server.Port), server.CrcTable), 10), 10, 64)
	server.ServerID = sid
	if cluster.Conf.TunnelHost != "" {
		go server.Tunnel()
	}

	server.SetCredential(url, user, pass)
	server.ReplicationSourceName = cluster.Conf.MainConn

	server.HaveSemiSync = true
	server.HaveInnodbTrxCommit = true
	server.HaveChecksum = true
	server.HaveBinlogSync = true
	server.HaveBinlogRow = true
	server.HaveBinlogAnnotate = true
	server.HaveBinlogCompress = true
	server.HaveBinlogSlowqueries = true
	server.MxsHaveGtid = false
	// consider all nodes are maxscale to avoid sending command until discoverd
	server.IsRelay = false
	server.IsMaxscale = true
	server.IsDelayed = server.IsInDelayedHost()
	server.State = stateSuspect
	server.PrevState = stateSuspect
	server.Datadir = server.ClusterGroup.Conf.WorkingDir + "/" + server.ClusterGroup.Name + "/" + server.Host + "_" + server.Port
	if _, err := os.Stat(server.Datadir); os.IsNotExist(err) {
		os.MkdirAll(server.Datadir, os.ModePerm)
		os.MkdirAll(server.Datadir+"/log", os.ModePerm)
		os.MkdirAll(server.Datadir+"/var", os.ModePerm)
		os.MkdirAll(server.Datadir+"/init", os.ModePerm)
		os.MkdirAll(server.Datadir+"/bck", os.ModePerm)
	}

	errLogFile := server.Datadir + "/log/log_error.log"
	slowLogFile := server.Datadir + "/log/log_slow_query.log"
	if _, err := os.Stat(errLogFile); os.IsNotExist(err) {
		nofile, _ := os.OpenFile(errLogFile, os.O_WRONLY|os.O_CREATE, 0600)
		nofile.Close()
	}
	if _, err := os.Stat(slowLogFile); os.IsNotExist(err) {
		nofile, _ := os.OpenFile(slowLogFile, os.O_WRONLY|os.O_CREATE, 0600)
		nofile.Close()
	}
	server.ErrorLogTailer, _ = tail.TailFile(errLogFile, tail.Config{Follow: true, ReOpen: true})
	server.SlowLogTailer, _ = tail.TailFile(slowLogFile, tail.Config{Follow: true, ReOpen: true})
	server.ErrorLog = s18log.NewHttpLog(server.ClusterGroup.Conf.MonitorErrorLogLength)
	server.SlowLog = s18log.NewSlowLog(server.ClusterGroup.Conf.MonitorLongQueryLogLength)
	go server.ErrorLogWatcher()
	go server.SlowLogWatcher()
	server.SetIgnored(cluster.IsInIgnoredHosts(server))
	server.SetPreferedBackup(cluster.IsInPreferedBackupHosts(server))
	server.SetPrefered(cluster.IsInPreferedHosts(server))
	if server.ClusterGroup.Conf.MainSubordinatePgStream || server.ClusterGroup.Conf.MainSubordinatePgLogical {
		server.Conn, err = sqlx.Open("postgres", server.DSN)
	} else {
		server.Conn, err = sqlx.Open("mysql", server.DSN)
	}
	return server, err
}

func (server *ServerMonitor) Ping(wg *sync.WaitGroup) {

	defer wg.Done()

	if server.ClusterGroup.vmain != nil {
		if server.ClusterGroup.vmain.ServerID == server.ServerID {
			server.IsVirtualMain = true
		} else {
			server.IsVirtualMain = false
		}
	}
	var conn *sqlx.DB
	var err error
	switch server.ClusterGroup.Conf.CheckType {
	case "tcp":
		conn, err = server.GetNewDBConn()
	case "agent":
		var resp *http.Response
		resp, err = http.Get("http://" + server.Host + ":10001/check/")
		if resp.StatusCode != 200 {
			// if 404, consider server down or agent killed. Don't initiate anything
			err = fmt.Errorf("HTTP Response Code Error: %d", resp.StatusCode)
		}
	}
	// manage IP based DNS may failed if backend server as changed IP  try to resolv it and recreate new DSN
	//server.SetCredential(server.URL, server.User, server.Pass)
	// Handle failure cases here
	if err != nil {
		// Copy the last known server states or they will be cleared at next monitoring loop
		if server.State != stateFailed {
			server.ClusterGroup.sme.CopyOldStateFromUnknowServer(server.URL)
		}
		// server.ClusterGroup.LogPrintf(LvlDbg, "Failure detection handling for server %s %s", server.URL, err)
		// server.ClusterGroup.LogPrintf(LvlErr, "Failure detection handling for server %s %s", server.URL, err)

		if driverErr, ok := err.(*mysql.MySQLError); ok {
			//	server.ClusterGroup.LogPrintf(LvlDbg, "Driver Error %s %d ", server.URL, driverErr.Number)
			server.ClusterGroup.LogPrintf(LvlErr, "Driver Error %s %d ", server.URL, driverErr.Number)

			// access denied
			if driverErr.Number == 1045 {
				server.State = stateErrorAuth
				server.ClusterGroup.SetState("ERR00004", state.State{ErrType: LvlErr, ErrDesc: fmt.Sprintf(clusterError["ERR00004"], server.URL, err.Error()), ErrFrom: "SRV"})
				return
			}
		}
		if err != sql.ErrNoRows {
			server.FailCount++
			if server.ClusterGroup.main == nil {
				server.ClusterGroup.LogPrintf(LvlDbg, "Main not defined")
			}
			if server.ClusterGroup.main != nil && server.URL == server.ClusterGroup.main.URL {
				server.FailSuspectHeartbeat = server.ClusterGroup.sme.GetHeartbeats()
				if server.ClusterGroup.main.FailCount <= server.ClusterGroup.Conf.MaxFail {
					server.ClusterGroup.LogPrintf("INFO", "Main Failure detected! Retry %d/%d", server.ClusterGroup.main.FailCount, server.ClusterGroup.Conf.MaxFail)
				}
				if server.FailCount >= server.ClusterGroup.Conf.MaxFail {
					if server.FailCount == server.ClusterGroup.Conf.MaxFail {
						server.ClusterGroup.LogPrintf("INFO", "Declaring db main as failed %s", server.URL)
					}
					server.ClusterGroup.main.State = stateFailed
					server.DelWaitStopCookie()
				} else {
					server.ClusterGroup.main.State = stateSuspect

				}
			} else {
				// not the main
				server.ClusterGroup.LogPrintf(LvlDbg, "Failure detection of no main FailCount %d MaxFail %d", server.FailCount, server.ClusterGroup.Conf.MaxFail)
				if server.FailCount >= server.ClusterGroup.Conf.MaxFail {
					if server.FailCount == server.ClusterGroup.Conf.MaxFail {
						server.ClusterGroup.LogPrintf("INFO", "Declaring subordinate db %s as failed", server.URL)
						server.State = stateFailed
						server.DelWaitStopCookie()
						// remove from subordinate list
						server.delete(&server.ClusterGroup.subordinates)
						if server.Replications != nil {
							server.LastSeenReplications = server.Replications
						}
						server.Replications = nil
					}
				} else {
					server.State = stateSuspect
				}
			}
		}
		// Send alert if state has changed
		if server.PrevState != server.State {
			//if cluster.Conf.Verbose {
			server.ClusterGroup.LogPrintf(LvlDbg, "Server %s state changed from %s to %s", server.URL, server.PrevState, server.State)
			if server.State != stateSuspect {
				server.ClusterGroup.LogPrintf("ALERT", "Server %s state changed from %s to %s", server.URL, server.PrevState, server.State)
				server.ClusterGroup.backendStateChangeProxies()
				server.SendAlert()
				server.ProcessFailedSubordinate()
			}
		}
		if server.PrevState != server.State {
			server.PrevState = server.State
		}
		return
	}

	// From here we have a new connection
	// We will affect it or closing it

	if server.ClusterGroup.sme.IsInFailover() {
		conn.Close()
		server.ClusterGroup.LogPrintf(LvlDbg, "Inside failover, skiping refresh")
		return
	}
	// reaffect a global DB pool object if we never get it , ex dynamic seeding
	if server.Conn == nil {
		server.Conn = conn
		server.ClusterGroup.LogPrintf(LvlInfo, "Assigning a global connection on server %s", server.URL)
		return
	}
	err = server.Refresh()
	if err != nil {
		// reaffect a global DB pool object if we never get it , ex dynamic seeding
		server.Conn = conn
		server.ClusterGroup.LogPrintf(LvlInfo, "Server refresh failed but ping connect %s", err)
		return
	}
	defer conn.Close()

	// For orchestrator to trigger a start via tracking state URL
	if server.PrevState == stateFailed {
		server.DelWaitStartCookie()
		server.DelRestartCookie()
	}
	// Reset FailCount
	if (server.State != stateFailed && server.State != stateErrorAuth && server.State != stateSuspect) && (server.FailCount > 0) /*&& (((server.ClusterGroup.sme.GetHeartbeats() - server.FailSuspectHeartbeat) * server.ClusterGroup.Conf.MonitoringTicker) > server.ClusterGroup.Conf.FailResetTime)*/ {
		server.FailCount = 0
		server.FailSuspectHeartbeat = 0
	}

	var ss dbhelper.SubordinateStatus
	ss, _, errss := dbhelper.GetSubordinateStatus(server.Conn, server.ClusterGroup.Conf.MainConn, server.DBVersion)
	// We have no replicatieon can this be the old main
	//  1617 is no multi source channel found
	noChannel := false
	if errss != nil {
		if strings.Contains(errss.Error(), "1617") {
			// This is a special case when using muti source there is a error instead of empty resultset when no replication is defined on channel
			//	server.ClusterGroup.LogPrintf(LvlInfo, " server: %s replication no channel err 1617 %s ", server.URL, errss)
			noChannel = true
		}
	}
	if errss == sql.ErrNoRows || noChannel {
		// If we reached this stage with a previously failed server, reintroduce
		// it as unconnected server.
		if server.PrevState == stateFailed || server.PrevState == stateErrorAuth {
			server.ClusterGroup.LogPrintf(LvlDbg, "State comparison reinitialized failed server %s as unconnected", server.URL)
			if server.ClusterGroup.Conf.ReadOnly && server.HaveWsrep == false && server.ClusterGroup.IsDiscovered() {
				if server.ClusterGroup.main != nil {
					if server.ClusterGroup.Status == ConstMonitorActif && server.ClusterGroup.main.Id != server.Id && !server.ClusterGroup.IsInIgnoredReadonly(server) {
						server.ClusterGroup.LogPrintf(LvlInfo, "Setting Read Only on unconnected server %s as active monitor and other main is discovered", server.URL)
						server.SetReadOnly()
					} else if server.ClusterGroup.Status == ConstMonitorStandby && server.ClusterGroup.Conf.Arbitration && !server.ClusterGroup.IsInIgnoredReadonly(server) {
						server.ClusterGroup.LogPrintf(LvlInfo, "Setting Read Only on unconnected server %s as a standby monitor ", server.URL)
						server.SetReadOnly()
					}
				}
			}
			//if server.ClusterGroup.GetTopology() != topoMultiMainWsrep {
			server.State = stateUnconn
			//}
			server.FailCount = 0
			server.ClusterGroup.backendStateChangeProxies()
			server.SendAlert()
			if server.ClusterGroup.Conf.Autorejoin && server.ClusterGroup.IsActive() {
				server.RejoinMain()
			} else {
				server.ClusterGroup.LogPrintf("INFO", "Auto Rejoin is disabled")
			}

		} else if server.State != stateMain && server.PrevState != stateUnconn {
			// Main will never get discovery in topology if it does not get unconnected first it default to suspect
			if server.ClusterGroup.GetTopology() != topoMultiMainWsrep {
				server.State = stateUnconn
				server.ClusterGroup.LogPrintf(LvlDbg, "State unconnected set by non-main rule on server %s", server.URL)
			}
			if server.ClusterGroup.Conf.ReadOnly && server.HaveWsrep == false && server.ClusterGroup.IsDiscovered() && !server.ClusterGroup.IsInIgnoredReadonly(server) {
				server.ClusterGroup.LogPrintf(LvlInfo, "Setting Read Only on unconnected server: %s no main state and replication found", server.URL)
				server.SetReadOnly()
			}

			if server.State != stateSuspect {
				server.ClusterGroup.backendStateChangeProxies()
				server.SendAlert()
			}
		}

	} else if server.ClusterGroup.IsActive() && errss == nil && (server.PrevState == stateFailed) {

		server.rejoinSubordinate(ss)
	}

	if server.PrevState != server.State {
		server.PrevState = server.State
		if server.PrevState != stateSuspect {
			server.ClusterGroup.backendStateChangeProxies()
			server.SendAlert()
		}
	}
}

func (server *ServerMonitor) ProcessFailedSubordinate() {

	if server.State == stateSubordinateErr {
		if server.ClusterGroup.Conf.ReplicationErrorScript != "" {
			server.ClusterGroup.LogPrintf("INFO", "Calling replication error script")
			var out []byte
			out, err := exec.Command(server.ClusterGroup.Conf.ReplicationErrorScript, server.URL, server.PrevState, server.State).CombinedOutput()
			if err != nil {
				server.ClusterGroup.LogPrintf("ERROR", "%s", err)
			}
			server.ClusterGroup.LogPrintf("INFO", "Replication error script complete:", string(out))
		}
		if server.HasReplicationSQLThreadRunning() && server.ClusterGroup.Conf.ReplicationRestartOnSQLErrorMatch != "" {
			ss, err := server.GetSubordinateStatus(server.ReplicationSourceName)
			if err != nil {
				return
			}
			matched, err := regexp.Match(server.ClusterGroup.Conf.ReplicationRestartOnSQLErrorMatch, []byte(ss.LastSQLError.String))
			if err != nil {
				server.ClusterGroup.LogPrintf("ERROR", "Rexep failed replication-restart-on-sqlerror-match %s %s", server.ClusterGroup.Conf.ReplicationRestartOnSQLErrorMatch, err)
			} else if matched {
				server.ClusterGroup.LogPrintf("INFO", "Rexep restart subordinate  %s  matching: %s", server.ClusterGroup.Conf.ReplicationRestartOnSQLErrorMatch, ss.LastSQLError.String)
				server.SkipReplicationEvent()
				server.StartSubordinate()
				server.ClusterGroup.LogPrintf("INFO", "Skip event and restart subordinate on %s", server.URL)
			}
		}
	}
}

// Refresh a server object
func (server *ServerMonitor) Refresh() error {
	var err error
	if server.Conn == nil {
		return errors.New("Connection is nil, server unreachable")
	}
	if server.Conn.Unsafe() == nil {
		//	server.State = stateFailed
		return errors.New("Connection is unsafe, server unreachable")
	}

	err = server.Conn.Ping()
	if err != nil {
		return err
	}

	if server.ClusterGroup.Conf.MxsBinlogOn {
		mxsversion, _ := dbhelper.GetMaxscaleVersion(server.Conn)
		if mxsversion != "" {
			server.ClusterGroup.LogPrintf(LvlInfo, "Found Maxscale")
			server.IsMaxscale = true
			server.IsRelay = true
			server.MxsVersion = dbhelper.MariaDBVersion(mxsversion)
			server.State = stateRelay
		} else {
			server.IsMaxscale = false
		}
	} else {
		server.IsMaxscale = false
	}

	if !(server.ClusterGroup.Conf.MxsBinlogOn && server.IsMaxscale) {
		// maxscale don't support show variables
		server.PrevMonitorTime = server.MonitorTime
		server.MonitorTime = time.Now().Unix()
		logs := ""
		server.DBVersion, logs, err = dbhelper.GetDBVersion(server.Conn)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlErr, "Could not get database version %s %s", server.URL, err)

		server.Variables, logs, err = dbhelper.GetVariables(server.Conn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlErr, "Could not get database variables %s %s", server.URL, err)
		if err != nil {
			return nil
		}
		if !server.DBVersion.IsPPostgreSQL() {

			server.HaveEventScheduler = server.HasEventScheduler()
			server.Strict = server.Variables["GTID_STRICT_MODE"]
			server.ReadOnly = server.Variables["READ_ONLY"]
			server.LongQueryTime = server.Variables["LONG_QUERY_TIME"]
			server.LogOutput = server.Variables["LOG_OUTPUT"]
			server.SlowQueryLog = server.Variables["SLOW_QUERY_LOG"]
			server.HaveReadOnly = server.HasReadOnly()
			server.HaveBinlog = server.HasBinlog()
			server.HaveBinlogRow = server.HasBinlogRow()
			server.HaveBinlogAnnotate = server.HasBinlogRowAnnotate()
			server.HaveBinlogSync = server.HasBinlogDurable()
			server.HaveBinlogCompress = server.HasBinlogCompress()
			server.HaveBinlogSubordinateUpdates = server.HasBinlogSubordinateUpdates()
			server.HaveBinlogSlowqueries = server.HasBinlogSlowSubordinateQueries()
			server.HaveGtidStrictMode = server.HasGtidStrictMode()
			server.HaveInnodbTrxCommit = server.HasInnoDBRedoLogDurable()
			server.HaveChecksum = server.HasInnoDBChecksum()
			server.HaveWsrep = server.HasWsrep()
			server.HaveSlowQueryLog = server.HasLogSlowQuery()
			server.HavePFS = server.HasLogPFS()
			if server.HavePFS {
				server.HavePFSSlowQueryLog = server.HasLogPFSSlowQuery()
			}
			server.HaveMySQLGTID = server.HasMySQLGTID()
			server.RelayLogSize, _ = strconv.ParseUint(server.Variables["RELAY_LOG_SPACE_LIMIT"], 10, 64)

			if server.DBVersion.IsMariaDB() {
				server.GTIDBinlogPos = gtid.NewList(server.Variables["GTID_BINLOG_POS"])
				server.CurrentGtid = gtid.NewList(server.Variables["GTID_CURRENT_POS"])
				server.SubordinateGtid = gtid.NewList(server.Variables["GTID_SLAVE_POS"])

			} else {
				server.GTIDBinlogPos = gtid.NewMySQLList(server.Variables["GTID_EXECUTED"])
				server.GTIDExecuted = server.Variables["GTID_EXECUTED"]
				server.CurrentGtid = gtid.NewMySQLList(server.Variables["GTID_EXECUTED"])
				server.SubordinateGtid = gtid.NewList(server.Variables["GTID_SLAVE_POS"])
			}

			var sid uint64
			sid, err = strconv.ParseUint(server.Variables["SERVER_ID"], 10, 64)
			if err != nil {
				server.ClusterGroup.LogPrintf(LvlErr, "Could not parse server_id, reason: %s", err)
			}
			server.ServerID = uint64(sid)

			server.EventStatus, logs, err = dbhelper.GetEventStatus(server.Conn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get events status %s %s", server.URL, err)
			if err != nil {
				server.ClusterGroup.SetState("ERR00073", state.State{ErrType: LvlErr, ErrDesc: fmt.Sprintf(clusterError["ERR00073"], server.URL), ErrFrom: "MON"})
			}
			if server.ClusterGroup.sme.GetHeartbeats()%30 == 0 {
				server.CheckPrivileges()
			}

		} // end not postgress

		// get Users
		server.Users, logs, err = dbhelper.GetUsers(server.Conn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get database users %s %s", server.URL, err)
		if server.ClusterGroup.Conf.MonitorScheduler {
			server.JobsCheckRunning()
		}

		if server.ClusterGroup.Conf.MonitorProcessList {
			server.FullProcessList, logs, err = dbhelper.GetProcesslist(server.Conn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get process %s %s", server.URL, err)
			if err != nil {
				server.ClusterGroup.SetState("ERR00075", state.State{ErrType: LvlErr, ErrDesc: fmt.Sprintf(clusterError["ERR00075"], err), ServerUrl: server.URL, ErrFrom: "MON"})
			}
		}
	}
	if server.InCaptureMode {
		server.ClusterGroup.SetState("WARN0085", state.State{ErrType: LvlInfo, ErrDesc: fmt.Sprintf(clusterError["WARN0085"], server.URL), ServerUrl: server.URL, ErrFrom: "MON"})
	}
	// SHOW MASTER STATUS
	logs := ""
	server.MainStatus, logs, err = dbhelper.GetMainStatus(server.Conn, server.DBVersion)
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get main status %s %s", server.URL, err)
	if err != nil {
		// binary log might be closed for that server
	} else {
		server.BinaryLogFile = server.MainStatus.File
		if server.BinaryLogFilePrevious != "" && server.BinaryLogFilePrevious != server.BinaryLogFile {
			server.BinaryLogFiles, logs, err = dbhelper.GetBinaryLogs(server.Conn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get binary log files %s %s", server.URL, err)
			if server.BinaryLogFilePrevious != "" {
				server.JobBackupBinlog(server.BinaryLogFilePrevious)
				go server.JobBackupBinlogPurge(server.BinaryLogFilePrevious)
			}
		}
		server.BinaryLogFilePrevious = server.BinaryLogFile
		server.BinaryLogPos = strconv.FormatUint(uint64(server.MainStatus.Position), 10)
	}

	if !server.DBVersion.IsPPostgreSQL() {
		server.BinlogDumpThreads, logs, err = dbhelper.GetBinlogDumpThreads(server.Conn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get binoDumpthreads status %s %s", server.URL, err)
		if err != nil {
			server.ClusterGroup.SetState("ERR00014", state.State{ErrType: LvlErr, ErrDesc: fmt.Sprintf(clusterError["ERR00014"], server.URL, err), ServerUrl: server.URL, ErrFrom: "CONF"})
		}

		if server.ClusterGroup.Conf.MonitorInnoDBStatus {
			// SHOW ENGINE INNODB STATUS
			server.EngineInnoDB, logs, err = dbhelper.GetEngineInnoDBVariables(server.Conn)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get engine innodb status %s %s", server.URL, err)
		}
		if server.ClusterGroup.Conf.MonitorPFS {
			// GET PFS query digest
			server.PFSQueries, logs, err = dbhelper.GetQueries(server.Conn)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get queries %s %s", server.URL, err)
		}
		if server.HaveDiskMonitor {
			server.Disks, logs, err = dbhelper.GetDisks(server.Conn, server.DBVersion)
		}
		if server.ClusterGroup.Conf.MonitorScheduler {
			server.CheckDisks()
		}
		if server.HasLogsInSystemTables() {
			go server.GetSlowLogTable()
		}

	} // End not PG

	// Set channel source name is dangerous with multi cluster

	// SHOW SLAVE STATUS

	if !(server.ClusterGroup.Conf.MxsBinlogOn && server.IsMaxscale) && server.DBVersion.IsMariaDB() || server.DBVersion.IsPPostgreSQL() {
		server.Replications, logs, err = dbhelper.GetAllSubordinatesStatus(server.Conn, server.DBVersion)
		if len(server.Replications) > 0 && err == nil && server.DBVersion.IsPPostgreSQL() && server.ReplicationSourceName == "" {
			//setting first subscription if we don't have one
			server.ReplicationSourceName = server.Replications[0].ConnectionName.String
		}
	} else {
		server.Replications, logs, err = dbhelper.GetChannelSubordinateStatus(server.Conn, server.DBVersion)
	}
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get subordinates status %s %s", server.URL, err)

	// select a replication status get an err if repliciations array is empty
	server.SubordinateStatus, err = server.GetSubordinateStatus(server.ReplicationSourceName)
	if err != nil {
		// Do not reset  server.MainServerID = 0 as we may need it for recovery
		server.IsSubordinate = false
	} else {

		server.IsSubordinate = true
		if server.DBVersion.IsPPostgreSQL() {
			//PostgresQL as no server_id concept mimic via internal server id for topology detection
			var sid uint64
			sid, err = strconv.ParseUint(strconv.FormatUint(crc64.Checksum([]byte(server.SubordinateStatus.MainHost.String+server.SubordinateStatus.MainPort.String), server.CrcTable), 10), 10, 64)
			if err != nil {
				server.ClusterGroup.LogPrintf(LvlWarn, "PG Could not assign server_id s", err)
			}
			server.SubordinateStatus.MainServerID = sid
			for i := range server.Replications {
				server.Replications[i].MainServerID = sid
			}

			server.SubordinateGtid = gtid.NewList(server.SubordinateStatus.GtidSubordinatePos.String)

		} else {
			if server.SubordinateStatus.UsingGtid.String == "Subordinate_Pos" || server.SubordinateStatus.UsingGtid.String == "Current_Pos" {
				server.HaveMariaDBGTID = true
			} else {
				server.HaveMariaDBGTID = false
			}
			if server.DBVersion.IsMySQLOrPerconaGreater57() && server.HasGTIDReplication() {
				server.SubordinateGtid = gtid.NewList(server.SubordinateStatus.ExecutedGtidSet.String)
			}
		}
	}
	server.ReplicationHealth = server.CheckReplication()
	// if MaxScale exit at fetch variables and status part as not supported

	if server.ClusterGroup.Conf.MxsBinlogOn && server.IsMaxscale {
		return nil
	}
	server.PrevStatus = server.Status

	server.Status, logs, _ = dbhelper.GetStatus(server.Conn, server.DBVersion)
	//server.ClusterGroup.LogPrintf("ERROR: %s %s %s", su["RPL_SEMI_SYNC_MASTER_STATUS"], su["RPL_SEMI_SYNC_SLAVE_STATUS"], server.URL)
	if server.Status["RPL_SEMI_SYNC_MASTER_STATUS"] == "" || server.Status["RPL_SEMI_SYNC_SLAVE_STATUS"] == "" {
		server.HaveSemiSync = false
	} else {
		server.HaveSemiSync = true
	}
	if server.Status["RPL_SEMI_SYNC_MASTER_STATUS"] == "ON" {
		server.SemiSyncMainStatus = true
	} else {
		server.SemiSyncMainStatus = false
	}
	if server.Status["RPL_SEMI_SYNC_SLAVE_STATUS"] == "ON" {
		server.SemiSyncSubordinateStatus = true
	} else {
		server.SemiSyncSubordinateStatus = false
	}

	if server.Status["WSREP_LOCAL_STATE"] == "4" {
		server.IsWsrepSync = true
	} else {
		server.IsWsrepSync = false
	}
	if server.Status["WSREP_LOCAL_STATE"] == "2" {
		server.IsWsrepDonor = true
	} else {
		server.IsWsrepDonor = false
	}
	if server.Status["WSREP_CLUSTER_STATUS"] == "PRIMARY" {
		server.IsWsrepPrimary = true
	} else {
		server.IsWsrepPrimary = false
	}
	if len(server.PrevStatus) > 0 {
		qps, _ := strconv.ParseInt(server.Status["QUERIES"], 10, 64)
		prevqps, _ := strconv.ParseInt(server.PrevStatus["QUERIES"], 10, 64)
		if server.MonitorTime-server.PrevMonitorTime > 0 {
			server.QPS = (qps - prevqps) / (server.MonitorTime - server.PrevMonitorTime)
		}
	}

	if server.HasHighNumberSlowQueries() {
		server.ClusterGroup.SetState("WARN0088", state.State{ErrType: LvlInfo, ErrDesc: fmt.Sprintf(clusterError["WARN0088"], server.URL), ServerUrl: server.URL, ErrFrom: "MON"})
	}
	// monitor plulgins plugins
	if !server.DBVersion.IsPPostgreSQL() {
		if server.ClusterGroup.sme.GetHeartbeats()%60 == 0 && !server.DBVersion.IsPPostgreSQL() {
			server.Plugins, logs, err = dbhelper.GetPlugins(server.Conn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get plugins  %s %s", server.URL, err)
			server.HaveMetaDataLocksLog = server.HasInstallPlugin("METADATA_LOCK_INFO")
			server.HaveQueryResponseTimeLog = server.HasInstallPlugin("QUERY_RESPONSE_TIME")
			server.HaveDiskMonitor = server.HasInstallPlugin("DISK")
			server.HaveSQLErrorLog = server.HasInstallPlugin("SQL_ERROR_LOG")
		}
		if server.HaveMetaDataLocksLog {
			server.MetaDataLocks, logs, err = dbhelper.GetMetaDataLock(server.Conn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get Metat data locks  %s %s", server.URL, err)
		}
	}
	server.CheckMaxConnections()

	// Initialize graphite monitoring
	if server.ClusterGroup.Conf.GraphiteMetrics {
		go server.SendDatabaseStats()
	}
	return nil
}

/* Handles write freeze and existing transactions on a server */
func (server *ServerMonitor) freeze() bool {
	logs, err := dbhelper.SetReadOnly(server.Conn, true)
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Freeze", LvlInfo, "Could not set %s as read-only: %s", server.URL, err)
	if err != nil {
		return false
	}
	for i := server.ClusterGroup.Conf.SwitchWaitKill; i > 0; i -= 500 {
		threads, logs, err := dbhelper.CheckLongRunningWrites(server.Conn, 0)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Freeze", LvlErr, "Could not check long running Writes %s as read-only: %s", server.URL, err)
		if threads == 0 {
			break
		}
		server.ClusterGroup.LogPrintf(LvlInfo, "Waiting for %d write threads to complete on %s", threads, server.URL)
		time.Sleep(500 * time.Millisecond)
	}
	server.maxConn, logs, err = dbhelper.GetVariableByName(server.Conn, "MAX_CONNECTIONS", server.DBVersion)
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Freeze", LvlErr, "Could not get max_connections value on demoted leader")
	if err != nil {

	} else {
		if server.ClusterGroup.Conf.SwitchDecreaseMaxConn {
			logs, err := dbhelper.SetMaxConnections(server.Conn, strconv.FormatInt(server.ClusterGroup.Conf.SwitchDecreaseMaxConnValue, 10), server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Freeze", LvlErr, "Could not set max_connections to 1 on demoted leader %s %s", server.URL, err)
		}
	}
	server.ClusterGroup.LogPrintf("INFO", "Terminating all threads on %s", server.URL)
	dbhelper.KillThreads(server.Conn, server.DBVersion)
	return true
}

func (server *ServerMonitor) ReadAllRelayLogs() error {

	server.ClusterGroup.LogPrintf(LvlInfo, "Reading all relay logs on %s", server.URL)
	if server.DBVersion.IsMariaDB() && server.HaveMariaDBGTID {
		ss, logs, err := dbhelper.GetMSubordinateStatus(server.Conn, "", server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "ReadAllRelayLogs", LvlErr, "Could not get subordinate status %s %s", server.URL, err)
		if err != nil {
			return err
		}
		server.Refresh()
		myGtid_IO_Pos := gtid.NewList(ss.GtidIOPos.String)
		myGtid_Subordinate_Pos := server.SubordinateGtid
		//myGtid_Subordinate_Pos := gtid.NewList(ss.GtidSubordinatePos.String)
		//https://jira.mariadb.org/browse/MDEV-14182

		for myGtid_Subordinate_Pos.Equal(myGtid_IO_Pos) == false && ss.UsingGtid.String != "" && ss.GtidSubordinatePos.String != "" && server.State != stateFailed {
			server.Refresh()
			ss, logs, err = dbhelper.GetMSubordinateStatus(server.Conn, server.ClusterGroup.Conf.MainConn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "ReadAllRelayLogs", LvlErr, "Could not get subordinate status %s %s", server.URL, err)

			if err != nil {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			myGtid_IO_Pos = gtid.NewList(ss.GtidIOPos.String)
			myGtid_Subordinate_Pos = server.SubordinateGtid

			server.ClusterGroup.LogPrintf(LvlInfo, "Waiting sync IO_Pos:%s, Subordinate_Pos:%s", myGtid_IO_Pos.Sprint(), myGtid_Subordinate_Pos.Sprint())
		}
	} else {
		ss, logs, err := dbhelper.GetSubordinateStatus(server.Conn, server.ClusterGroup.Conf.MainConn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "ReadAllRelayLogs", LvlErr, "Could not get subordinate status %s %s", server.URL, err)
		if err != nil {
			return err
		}
		for true {
			server.ClusterGroup.LogPrintf(LvlInfo, "Waiting sync IO_Pos:%s/%s, Subordinate_Pos:%s %s", ss.MainLogFile, ss.ReadMainLogPos.String, ss.RelayMainLogFile, ss.ExecMainLogPos.String)
			if ss.MainLogFile == ss.RelayMainLogFile && ss.ReadMainLogPos == ss.ExecMainLogPos {
				break
			}
			ss, logs, err = dbhelper.GetSubordinateStatus(server.Conn, server.ClusterGroup.Conf.MainConn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "ReadAllRelayLogs", LvlErr, "Could not get subordinate status %s %s", server.URL, err)
			if err != nil {
				return err
			}
			if strings.Contains(ss.SubordinateSQLRunningState.String, "Subordinate has read all relay log") {
				break
			}

			time.Sleep(500 * time.Millisecond)
		}
	}
	return nil
}

func (server *ServerMonitor) LogReplPostion() {
	server.Refresh()
	server.ClusterGroup.LogPrintf(LvlInfo, "Server:%s Current GTID:%s Subordinate GTID:%s Binlog Pos:%s", server.URL, server.CurrentGtid.Sprint(), server.SubordinateGtid.Sprint(), server.GTIDBinlogPos.Sprint())
	return
}

func (server *ServerMonitor) Close() {
	server.Conn.Close()
	return
}

func (server *ServerMonitor) writeState() error {
	server.LogReplPostion()
	f, err := os.Create("/tmp/repmgr.state")
	if err != nil {
		return err
	}
	_, err = f.WriteString(server.GTIDBinlogPos.Sprint())
	if err != nil {
		return err
	}
	return nil
}

func (server *ServerMonitor) delete(sl *serverList) {
	lsm := *sl
	for k, s := range lsm {
		if server.URL == s.URL {
			lsm[k] = lsm[len(lsm)-1]
			lsm[len(lsm)-1] = nil
			lsm = lsm[:len(lsm)-1]
			break
		}
	}
	*sl = lsm
}

func (server *ServerMonitor) StopSubordinate() (string, error) {
	return dbhelper.StopSubordinate(server.Conn, server.ClusterGroup.Conf.MainConn, server.DBVersion)
}

func (server *ServerMonitor) StartSubordinate() (string, error) {
	return dbhelper.StartSubordinate(server.Conn, server.ClusterGroup.Conf.MainConn, server.DBVersion)

}

func (server *ServerMonitor) ResetMain() (string, error) {
	return dbhelper.ResetMain(server.Conn, server.ClusterGroup.Conf.MainConn, server.DBVersion)
}

func (server *ServerMonitor) ResetPFSQueries() error {
	return server.ExecQueryNoBinLog("truncate performance_schema.events_statements_summary_by_digest")
}

func (server *ServerMonitor) StopSubordinateIOThread() (string, error) {
	return dbhelper.StopSubordinateIOThread(server.Conn, server.ClusterGroup.Conf.MainConn, server.DBVersion)
}

func (server *ServerMonitor) StopSubordinateSQLThread() (string, error) {
	return dbhelper.StopSubordinateSQLThread(server.Conn, server.ClusterGroup.Conf.MainConn, server.DBVersion)
}

func (server *ServerMonitor) ResetSubordinate() (string, error) {
	return dbhelper.ResetSubordinate(server.Conn, true, server.ClusterGroup.Conf.MainConn, server.DBVersion)
}

func (server *ServerMonitor) FlushLogs() (string, error) {
	return dbhelper.FlushLogs(server.Conn)
}

func (server *ServerMonitor) FlushTables() (string, error) {
	return dbhelper.FlushTables(server.Conn)
}

func (server *ServerMonitor) Uprovision() {
	server.ClusterGroup.OpenSVCUnprovisionDatabaseService(server)
}

func (server *ServerMonitor) Provision() {
	server.ClusterGroup.OpenSVCProvisionDatabaseService(server)
}

func (server *ServerMonitor) SkipReplicationEvent() {
	server.StopSubordinate()
	dbhelper.SkipBinlogEvent(server.Conn, server.ClusterGroup.Conf.MainConn, server.DBVersion)
	server.StartSubordinate()
}

func (server *ServerMonitor) KillThread(id string) (string, error) {
	return dbhelper.KillThread(server.Conn, id, server.DBVersion)
}

func (server *ServerMonitor) KillQuery(id string) (string, error) {
	return dbhelper.KillQuery(server.Conn, id, server.DBVersion)
}

func (server *ServerMonitor) ExecQueryNoBinLog(query string) error {
	Conn, err := server.GetNewDBConn()
	if err != nil {
		server.ClusterGroup.LogPrintf(LvlErr, "Error connection in exec query no log %s", err)
		return err
	}
	defer Conn.Close()
	_, err = Conn.Exec("set sql_log_bin=0")
	if err != nil {
		server.ClusterGroup.LogPrintf(LvlErr, "Error disabling binlog %s", err)
		return err
	}
	_, err = Conn.Exec(query)
	if err != nil {
		server.ClusterGroup.LogPrintf(LvlErr, "Error query %s %s", query, err)
		return err
	}
	return err
}

func (server *ServerMonitor) InstallPlugin(name string) error {
	val, ok := server.Plugins[name]

	if !ok {
		return errors.New("Plugin not loaded")
	} else {
		if val.Status == "NOT INSTALLED" {
			query := "INSTALL PLUGIN " + name + " SONAME '" + val.Library.String + "'"
			err := server.ExecQueryNoBinLog(query)
			if err != nil {
				return err
			}
			val.Status = "ACTIVE"
			server.Plugins[name] = val
		} else {
			return errors.New("Already Install Plugin")
		}
	}
	return nil
}

func (server *ServerMonitor) UnInstallPlugin(name string) error {
	val, ok := server.Plugins[name]
	if !ok {
		return errors.New("Plugin not loaded")
	} else {
		if val.Status == "ACTIVE" {
			query := "UNINSTALL PLUGIN " + name
			err := server.ExecQueryNoBinLog(query)
			if err != nil {
				return err
			}
			val.Status = "NOT INSTALLED"
			server.Plugins[name] = val
		} else {
			return errors.New("Already not installed Plugin")
		}
	}
	return nil
}

func (server *ServerMonitor) Capture() error {

	if server.InCaptureMode {
		return nil
	}

	go server.CaptureLoop(server.ClusterGroup.GetStateMachine().GetHeartbeats())
	go server.JobCapturePurge(server.ClusterGroup.Conf.WorkingDir+"/"+server.ClusterGroup.Name, server.ClusterGroup.Conf.MonitorCaptureFileKeep)
	return nil
}

func (server *ServerMonitor) CaptureLoop(start int64) {
	server.InCaptureMode = true

	type Save struct {
		ProcessList  []dbhelper.Processlist `json:"processlist"`
		InnoDBStatus string                 `json:"innodbstatus"`
		Status       map[string]string      `json:"status"`
		SubordinateSatus   []dbhelper.SubordinateStatus `json:"subordinatestatus"`
	}

	t := time.Now()
	logs := ""
	var err error
	for true {

		var clsave Save
		clsave.ProcessList,
			logs, err = dbhelper.GetProcesslist(server.Conn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "CaptureLoop", LvlErr, "Failed Processlist for server %s: %s ", server.URL, err)

		clsave.InnoDBStatus, logs, err = dbhelper.GetEngineInnoDBSatus(server.Conn)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "CaptureLoop", LvlErr, "Failed InnoDB Status for server %s: %s ", server.URL, err)
		clsave.Status, logs, err = dbhelper.GetStatus(server.Conn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "CaptureLoop", LvlErr, "Failed Status for server %s: %s ", server.URL, err)

		if !(server.ClusterGroup.Conf.MxsBinlogOn && server.IsMaxscale) && server.DBVersion.IsMariaDB() {
			clsave.SubordinateSatus, logs, err = dbhelper.GetAllSubordinatesStatus(server.Conn, server.DBVersion)
		} else {
			clsave.SubordinateSatus, logs, err = dbhelper.GetChannelSubordinateStatus(server.Conn, server.DBVersion)
		}
		server.ClusterGroup.LogSQL(logs, err, server.URL, "CaptureLoop", LvlErr, "Failed Subordinate Status for server %s: %s ", server.URL, err)

		saveJSON, _ := json.MarshalIndent(clsave, "", "\t")
		err := ioutil.WriteFile(server.ClusterGroup.Conf.WorkingDir+"/"+server.ClusterGroup.Name+"/capture_"+server.Name+"_"+t.Format("20060102150405")+".json", saveJSON, 0644)
		if err != nil {
			return
		}
		if server.ClusterGroup.GetStateMachine().GetHeartbeats() < start+5 {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	server.InCaptureMode = false
}

func (server *ServerMonitor) RotateSystemLogs() {
	server.ClusterGroup.LogPrintf(LvlInfo, "Log rotate on %s", server.URL)

	if server.HasLogsInSystemTables() && !server.IsDown() {
		if server.HasLogSlowQuery() {
			server.RotateTableToTime("mysql", "slow_log")
		}
		if server.HasLogGeneral() {
			server.RotateTableToTime("mysql", "general_log")
		}
	}
}

func (server *ServerMonitor) RotateTableToTime(database string, table string) {
	currentTime := time.Now()
	timeStampString := currentTime.Format("20060102150405")
	newtablename := table + "_" + timeStampString
	temptable := table + "_temp"
	query := "CREATE TABLE IF NOT EXISTS " + database + "." + temptable + " LIKE " + database + "." + table
	server.ExecQueryNoBinLog(query)
	query = "RENAME TABLE  " + database + "." + table + " TO " + database + "." + newtablename + " , " + database + "." + temptable + " TO " + database + "." + table
	server.ExecQueryNoBinLog(query)
	query = "select table_name from information_schema.tables where table_schema='" + database + "' and table_name like '" + table + "_%' order by table_name desc limit " + strconv.Itoa(server.ClusterGroup.Conf.SchedulerMaintenanceDatabaseLogsTableKeep) + ",100"
	cleantables := []string{}

	err := server.Conn.Select(&cleantables, query)
	if err != nil {
		return
	}
	for _, row := range cleantables {
		server.ExecQueryNoBinLog("DROP TABLE " + database + "." + row)
	}
}

func (server *ServerMonitor) WaitInnoDBPurge() error {
	query := "SET GLOBAL innodb_purge_rseg_truncate_frequency=1"
	server.ExecQueryNoBinLog(query)
	ct := 0
	for {
		if server.EngineInnoDB["history_list_lenght_inside_innodb"] == "0" {
			return nil
		}
		if ct == 1200 {
			return errors.New("Waiting to long for history_list_lenght_inside_innodb 0")
		}
	}
}

func (server *ServerMonitor) Shutdown() error {
	_, err := server.Conn.Exec("SHUTDOWN")
	if err != nil {
		server.ClusterGroup.LogPrintf("TEST", "Shutdown failed %s", err)
		return err
	}
	return nil
}

func (server *ServerMonitor) ChangeMainTo(main *ServerMonitor, main_use_gitd string) error {
	logs := ""
	var err error
	hasMyGTID := server.HasMySQLGTID()
	//mariadb

	if server.State != stateFailed && server.ClusterGroup.Conf.ForceSubordinateNoGtid == false && server.DBVersion.IsMariaDB() && server.DBVersion.Major >= 10 {
		main.Refresh()
		_, err = server.Conn.Exec("SET GLOBAL gtid_subordinate_pos = \"" + main.CurrentGtid.Sprint() + "\"")
		if err != nil {
			return err
		}
		logs, err = dbhelper.ChangeMain(server.Conn, dbhelper.ChangeMainOpt{
			Host:        main.Host,
			Port:        main.Port,
			User:        server.ClusterGroup.rplUser,
			Password:    server.ClusterGroup.rplPass,
			Retry:       strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatRetry),
			Heartbeat:   strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatTime),
			Mode:        main_use_gitd,
			Channel:     server.ClusterGroup.Conf.MainConn,
			IsDelayed:   server.IsDelayed,
			Delay:       strconv.Itoa(server.ClusterGroup.Conf.HostsDelayedTime),
			SSL:         server.ClusterGroup.Conf.ReplicationSSL,
			PostgressDB: server.PostgressDB,
		}, server.DBVersion)
		server.ClusterGroup.LogPrintf(LvlInfo, "Replication bootstrapped with %s as main", main.URL)
	} else if hasMyGTID && server.ClusterGroup.Conf.ForceSubordinateNoGtid == false {

		logs, err = dbhelper.ChangeMain(server.Conn, dbhelper.ChangeMainOpt{
			Host:        main.Host,
			Port:        main.Port,
			User:        server.ClusterGroup.rplUser,
			Password:    server.ClusterGroup.rplPass,
			Retry:       strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatRetry),
			Heartbeat:   strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatTime),
			Mode:        "MASTER_AUTO_POSITION",
			IsDelayed:   server.IsDelayed,
			Delay:       strconv.Itoa(server.ClusterGroup.Conf.HostsDelayedTime),
			SSL:         server.ClusterGroup.Conf.ReplicationSSL,
			Channel:     server.ClusterGroup.Conf.MainConn,
			PostgressDB: server.PostgressDB,
		}, server.DBVersion)
		server.ClusterGroup.LogPrintf(LvlInfo, "Replication bootstrapped with MySQL GTID replication style and %s as main", main.URL)

	} else {
		logs, err = dbhelper.ChangeMain(server.Conn, dbhelper.ChangeMainOpt{
			Host:        main.Host,
			Port:        main.Port,
			User:        server.ClusterGroup.rplUser,
			Password:    server.ClusterGroup.rplPass,
			Retry:       strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatRetry),
			Heartbeat:   strconv.Itoa(server.ClusterGroup.Conf.ForceSubordinateHeartbeatTime),
			Mode:        "POSITIONAL",
			Logfile:     main.BinaryLogFile,
			Logpos:      main.BinaryLogPos,
			Channel:     server.ClusterGroup.Conf.MainConn,
			IsDelayed:   server.IsDelayed,
			Delay:       strconv.Itoa(server.ClusterGroup.Conf.HostsDelayedTime),
			SSL:         server.ClusterGroup.Conf.ReplicationSSL,
			PostgressDB: server.PostgressDB,
		}, server.DBVersion)
		server.ClusterGroup.LogPrintf(LvlInfo, "Replication bootstrapped with old replication style and %s as main", main.URL)

	}
	if err != nil {
		server.ClusterGroup.LogSQL(logs, err, server.URL, "BootstrapReplication", LvlErr, "Replication can't be bootstrap for server %s with %s as main: %s ", server.URL, main.URL, err)
	}
	_, err = server.Conn.Exec("START SLAVE '" + server.ClusterGroup.Conf.MainConn + "'")
	if err != nil {
		err = errors.New(fmt.Sprintln("Can't start subordinate: ", err))
	}
	return err
}
