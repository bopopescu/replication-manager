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

func testFailoverSemisyncAutoRejoinFlashback(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	cluster.SetFailSync(false)
	cluster.SetInteractive(false)
	cluster.SetRplChecks(false)
	cluster.SetRejoin(true)
	cluster.SetRejoinFlashback(true)
	cluster.SetRejoinDump(false)

	SaveMainURL := cluster.GetMain().URL
	SaveMain := cluster.GetMain()
	//clusteruster.DelayAllSubordinates()
	cluster.PrepareBench()
	//go clusteruster.RunBench()
	go cluster.RunSysbench()
	time.Sleep(4 * time.Second)
	cluster.FailoverAndWait()
	/// give time to start the failover

	if cluster.GetMain().URL == SaveMainURL {
		cluster.LogPrintf("TEST", "Old main %s ==  Next main %s  ", SaveMainURL, cluster.GetMain().URL)

		return false
	}
	cluster.RunSysbench()
	wg2 := new(sync.WaitGroup)
	wg2.Add(1)
	go cluster.WaitRejoin(wg2)
	cluster.StartDatabaseService(SaveMain)
	wg2.Wait()
	SaveMain.ReadAllRelayLogs()
	if cluster.CheckTableConsistency("test.sbtest") != true {
		cluster.LogPrintf(LvlErr, "Inconsitant subordinate")

		return false
	}

	return true
}
