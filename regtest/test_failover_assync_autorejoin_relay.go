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

func testFailoverAssyncAutoRejoinRelay(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {
	cluster.SetMultiTierSubordinate(true)
	cluster.SetFailSync(false)
	cluster.SetInteractive(false)
	cluster.SetRplChecks(false)
	cluster.SetRejoin(true)
	cluster.SetRejoinFlashback(true)
	cluster.SetRejoinDump(false)
	cluster.DisableSemisync()
	SaveMain := cluster.GetMain()
	SaveMainURL := SaveMain.URL

	go cluster.RunSysbench()
	time.Sleep(4 * time.Second)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go cluster.WaitFailover(wg)
	cluster.StopDatabaseService(SaveMain)
	wg.Wait()
	/// give time to start the failover

	if cluster.GetMain().URL == SaveMainURL {
		cluster.LogPrintf("TEST", " Old main %s ==  Next main %s  ", SaveMainURL, cluster.GetMain().URL)

		return false
	}

	wg2 := new(sync.WaitGroup)
	wg2.Add(1)
	go cluster.WaitRejoin(wg2)
	cluster.StartDatabaseService(SaveMain)
	wg2.Wait()

	if cluster.CheckTableConsistency("test.sbtest") != true {
		cluster.LogPrintf(LvlErr, "Inconsitant subordinate")

		return false
	}
	time.Sleep(8 * time.Second)
	relay, _ := cluster.GetMainFromReplication(SaveMain)
	cluster.LogPrintf("TEST", "Pointing to relay %s", relay.URL)
	if relay == nil {
		cluster.LogPrintf("TEST", "Old main is not attach to Relay  ")

		return false
	}
	if relay.IsRelay == false {
		cluster.LogPrintf("TEST", "Old main is not attach to Relay  ")

		return false
	}

	return true
}
