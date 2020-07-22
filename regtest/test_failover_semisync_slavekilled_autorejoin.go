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

func testFailoverSemisyncSubordinatekilledAutoRejoin(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	cluster.SetFailSync(false)
	cluster.SetInteractive(false)
	cluster.SetRplChecks(false)
	cluster.SetRejoin(true)
	cluster.SetRejoinFlashback(true)
	cluster.SetRejoinDump(false)

	SaveMain := cluster.GetMain()
	SaveMainURL := SaveMain.URL
	//clusteruster.DelayAllSubordinates()
	killedSubordinate := cluster.GetSubordinates()[0]
	cluster.StopDatabaseService(killedSubordinate)

	time.Sleep(5 * time.Second)
	cluster.FailoverAndWait()

	if cluster.GetMain().URL == SaveMainURL {
		cluster.LogPrintf("TEST", "Old main %s ==  Next main %s  ", SaveMainURL, cluster.GetMain().URL)

		return false
	}
	cluster.PrepareBench()

	cluster.StartDatabaseService(killedSubordinate)
	time.Sleep(12 * time.Second)
	wg2 := new(sync.WaitGroup)
	wg2.Add(1)
	go cluster.WaitRejoin(wg2)
	cluster.StartDatabaseService(SaveMain)
	wg2.Wait()
	SaveMain.ReadAllRelayLogs()

	if killedSubordinate.HasSiblings(cluster.GetSubordinates()) == false {
		cluster.LogPrintf(LvlErr, "Not all subordinates pointing to main")

		return false
	}

	return true
}
