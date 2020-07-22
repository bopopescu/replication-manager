// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.

package regtest

import (
	"sync"
	"time"

	"github.com/signal18/replication-manager/cluster"
)

func testFailoverSemisyncAutoRejoinUnsafeMSXMSM(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	cluster.SetFailoverCtr(0)
	cluster.SetFailSync(false)
	cluster.SetInteractive(false)
	cluster.SetRplChecks(false)
	cluster.SetRejoin(true)
	cluster.SetRejoinFlashback(true)
	cluster.SetRejoinDump(true)
	cluster.EnableSemisync()
	cluster.SetFailTime(0)
	cluster.SetFailRestartUnsafe(true)
	cluster.SetBenchMethod("table")
	SaveMainURL := cluster.GetMain().URL
	SaveMain := cluster.GetMain()
	//clusteruster.DelayAllSubordinates()
	cluster.CleanupBench()
	cluster.PrepareBench()
	go cluster.RunBench()
	time.Sleep(4 * time.Second)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go cluster.WaitFailover(wg)
	cluster.StopDatabaseService(cluster.GetMain())
	wg.Wait()

	cluster.RunBench()

	if cluster.GetMain().URL == SaveMainURL {
		cluster.LogPrintf("TEST", "Old main %s ==  Next main %s  ", SaveMainURL, cluster.GetMain().URL)

		return false
	}
	wg2 := new(sync.WaitGroup)
	wg2.Add(1)
	go cluster.WaitRejoin(wg2)
	cluster.StartDatabaseService(SaveMain)
	wg2.Wait()
	//Recovered as subordinate first wait that it trigger main failover
	time.Sleep(5 * time.Second)

	for _, s := range cluster.GetSubordinates() {
		if s.IsReplicationBroken() {
			cluster.LogPrintf(LvlErr, "Subordinate  %s issue on replication", s.URL)

			return false
		}
	}
	time.Sleep(10 * time.Second)
	if cluster.ChecksumBench() != true {
		cluster.LogPrintf(LvlErr, "Inconsitant subordinate")

		return false
	}
	if len(cluster.GetServers()) == 2 && SaveMainURL == cluster.GetMain().URL {
		cluster.LogPrintf(LvlErr, "Unexpected main for 2 nodes cluster")
		return false
	}

	return true
}
