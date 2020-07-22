// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.

package regtest

import "github.com/signal18/replication-manager/cluster"

func testFailoverManual(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	cluster.SetFailSync(false)
	cluster.SetInteractive(true)
	cluster.SetRplChecks(true)
	SaveMain := cluster.GetMain()
	SaveMainURL := SaveMain.URL
	cluster.FailoverNow()
	if cluster.GetMain().URL != SaveMainURL {
		cluster.LogPrintf("TEST", " Old main %s !=  Next main %s  ", SaveMainURL, cluster.GetMain().URL)
		return false
	}

	return true
}
