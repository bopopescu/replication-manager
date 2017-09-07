// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <stephane@mariadb.com>
// This source code is licensed under the GNU General Public License, version 3.

package regtest

import (
	"time"

	"github.com/tanji/replication-manager/cluster"
)

func testFailoverAllSlavesDelayRplChecksNoSemiSync(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	err := cluster.DisableSemisync()
	if err != nil {
		cluster.LogPrintf("ERROR", "%s", err)
		return false
	}

	SaveMasterURL := cluster.GetMaster().URL
	cluster.DelayAllSlaves()
	cluster.LogPrintf("TEST", "Master is %s", cluster.GetMaster().URL)
	cluster.SetInteractive(false)
	cluster.SetFailLimit(5)
	cluster.SetFailTime(0)
	cluster.SetFailoverCtr(0)
	cluster.SetCheckFalsePositiveHeartbeat(false)
	cluster.SetRplChecks(true)
	cluster.SetRplMaxDelay(4)

	cluster.FailoverAndWait()

	cluster.LogPrintf("TEST", " New Master  %s ", cluster.GetMaster().URL)

	time.Sleep(2 * time.Second)
	if cluster.GetMaster().URL != SaveMasterURL {
		cluster.LogPrintf("ERROR", "Old master %s ==  New master %s  ", SaveMasterURL, cluster.GetMaster().URL)

		return false
	}

	return true
}
