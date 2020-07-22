package regtest

import (
	"sort"
	"strings"

	"github.com/signal18/replication-manager/cluster"
)

var tests = []string{
	"testSwitchoverAllSubordinatesDelayMultimainNoRplChecksNoSemiSync",
	"testSwitchoverLongTransactionNoRplCheckNoSemiSync",
	"testSwitchoverLongQueryNoRplCheckNoSemiSync",
	"testSwitchoverLongTrxWithoutCommitNoRplCheckNoSemiSync",
	"testSwitchoverReadOnlyNoRplCheck",
	"testSwitchoverNoReadOnlyNoRplCheck",
	"testSwitchover2TimesReplicationOkNoSemiSyncNoRplCheck",
	"testSwitchover2TimesReplicationOkSemiSyncNoRplCheck",
	"testSwitchoverBackPreferedMainNoRplCheckSemiSync",
	"testSwitchoverAllSubordinatesStopRplCheckNoSemiSync",
	"testSwitchoverAllSubordinatesStopNoSemiSyncNoRplCheck",
	"testSwitchoverAllSubordinatesDelayRplCheckNoSemiSync",
	"testSwitchoverAllSubordinatesDelayNoRplChecksNoSemiSync",
	"testFailoverSemisyncAutoRejoinSafeMSMXMS",
	"testFailoverSemisyncAutoRejoinSafeMSXMSM",
	"testFailoverSemisyncAutoRejoinSafeMSMXXXRMXMS",
	"testFailoverSemisyncAutoRejoinSafeMSMXXXRXSMS",
	"testFailoverSemisyncAutoRejoinUnsafeMSMXMS",
	"testFailoverSemisyncAutoRejoinUnsafeMSMXXXMXMS",
	"testFailoverSemisyncAutoRejoinUnsafeMSMXXXXMSM",
	"testFailoverSemisyncAutoRejoinUnsafeMSXMSM",
	"testFailoverSemisyncAutoRejoinUnsafeMSXMXXMXMS",
	"testFailoverSemisyncAutoRejoinUnsafeMSXMXXXMSM",
	"testFailoverSemisyncAutoRejoinUnsafeMSMXXXRMXMS",
	"testFailoverSemisyncAutoRejoinUnsafeMSMXXXRXMSM",
	"testFailoverAssyncAutoRejoinRelay",
	"testFailoverAssyncAutoRejoinNoGtid",
	"testFailoverAllSubordinatesDelayNoRplChecksNoSemiSync",
	"testFailoverAllSubordinatesDelayRplChecksNoSemiSync",
	"testFailoverNoRplChecksNoSemiSync",
	"testFailoverNoRplChecksNoSemiSyncMainHeartbeat",
	"testFailoverNumberFailureLimitReach",
	"testFailoverTimeNotReach",
	"testFailoverManual",
	"testFailoverAssyncAutoRejoinFlashback",
	"testFailoverSemisyncAutoRejoinFlashback",
	"testFailoverAssyncAutoRejoinNowrites",
	"testFailoverSemisyncAutoRejoinMSSXMSXXMSXMSSM",
	"testFailoverSemisyncAutoRejoinMSSXMSXXMXSMSSM",
	"testFailoverSemisyncSubordinatekilledAutoRejoin",
	"testSlaReplAllSubordinatesStopNoSemiSync",
	"testSlaReplAllSubordinatesDelayNoSemiSync",
}

const recoverTime = 8
const LvlErr = "ERROR"
const LvlInfo = "INFO"

type RegTest struct {
}

func (regtest *RegTest) GetTests() []string {
	return tests
}

func (regtest *RegTest) RunAllTests(cl *cluster.Cluster, test string) []cluster.Test {
	var allTests = map[string]cluster.Test{}
	conf := "semisync.cnf"

	var res bool
	cl.LogPrintf("TESTING : %s", test)
	var thistest cluster.Test
	thistest.ConfigFile = cl.GetConf().ConfigFile
	if test == "testFailoverManual" || test == "ALL" {

		thistest.Name = "testFailoverManual"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinSafeMSMXMS(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverManual"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinSafeMSMXMS" || test == "ALL" {

		thistest.Name = "testFailoverSemisyncAutoRejoinSafeMSMXMS"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinSafeMSMXMS(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinSafeMSMXMS"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinSafeMSXMSM" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinSafeMSXMSM"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinSafeMSXMSM(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinSafeMSXMSM"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinSafeMSMXXXRMXMS" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinSafeMSMXXXRMXMS"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinSafeMSMXXXRMXMS(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinSafeMSMXXXRMXMS"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinSafeMSMXXXRXSMS" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinSafeMSMXXXRXSMS"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinSafeMSMXXXRXSMS(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinSafeMSMXXXRXSMS"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinUnsafeMSMXMS" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinUnsafeMSMXMS"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinUnsafeMSMXMS(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinUnsafeMSMXMS"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinUnsafeMSMXXXMXMS" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinUnsafeMSMXXXMXMS"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinUnsafeMSMXXXMXMS(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinUnsafeMSMXXXMXMS"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinUnsafeMSMXXXXMSM" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinUnsafeMSMXXXXMSM"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinUnsafeMSMXXXXMSM(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinUnsafeMSMXXXXMSM"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinUnsafeMSXMSM" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinUnsafeMSXMSM"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinUnsafeMSXMSM(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinUnsafeMSXMSM"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinUnsafeMSXMXXMXMS" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinUnsafeMSXMXXMXMS"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinUnsafeMSXMXXMXMS(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinUnsafeMSXMXXMXMS"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinUnsafeMSXMXXXMSM" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinUnsafeMSXMXXXMSM"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinUnsafeMSXMXXXMSM(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinUnsafeMSXMXXXMSM"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinUnsafeMSMXXXRMXMS" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinUnsafeMSMXXRXMXMS"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinUnsafeMSMXXXRMXMS(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinUnsafeMSMXXRXMXMS"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinUnsafeMSMXXXRXMSM" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinUnsafeMSMXXXRXMSM"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinUnsafeMSMXXXRXMSM(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinUnsafeMSMXXXRXMSM"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}

	if test == "testFailoverAssyncAutoRejoinNoGtid" || test == "ALL" {
		thistest.Name = "testFailoverAssyncAutoRejoinNoGtid"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverAssyncAutoRejoinNoGtid(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverAssyncAutoRejoinNoGtid"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverAssyncAutoRejoinRelay" || test == "ALL" {
		thistest.Name = "testFailoverAssyncAutoRejoinRelay"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverAssyncAutoRejoinRelay(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverAssyncAutoRejoinRelay"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinMSSXMSXXMSXMSSM" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinMSSXMSXXMSXMSSM"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinMSSXMSXXMSXMSSM(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinMSSXMSXXMSXMSSM"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncAutoRejoinMSSXMSXXMXSMSSM" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinMSSXMSXXMXSMSSM"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinMSSXMSXXMXSMSSM(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinMSSXMSXXMXSMSSM"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverSemisyncSubordinatekilledAutoRejoin" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncSubordinatekilledAutoRejoin"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncSubordinatekilledAutoRejoin(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncSubordinatekilledAutoRejoin"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}

	if test == "testFailoverSemisyncAutoRejoinFlashback" || test == "ALL" {
		thistest.Name = "testFailoverSemisyncAutoRejoinFlashback"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverSemisyncAutoRejoinFlashback(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverSemisyncAutoRejoinFlashback"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverAssyncAutoRejoinFlashback" || test == "ALL" {
		thistest.Name = "testFailoverAssyncAutoRejoinFlashback"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverAssyncAutoRejoinFlashback(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverAssyncAutoRejoinFlashback"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverAssyncAutoRejoinNowrites" || test == "ALL" {
		thistest.Name = "testFailoverAssyncAutoRejoinNowrites"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverAssyncAutoRejoinNowrites(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverAssyncAutoRejoinNowrites"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverAssyncAutoRejoinDump" || test == "ALL" {
		thistest.Name = "testFailoverAssyncAutoRejoinDump"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverAssyncAutoRejoinDump(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverAssyncAutoRejoinDump"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverAllSubordinatesDelayMultimainNoRplChecksNoSemiSync" || test == "ALL" {
		thistest.Name = "testSwitchoverAllSubordinatesDelayMultimainNoRplChecksNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverAllSubordinatesDelayMultimainNoRplChecksNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
			cl.CloseTestCluster(conf, &thistest)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverAllSubordinatesDelayMultimainNoRplChecksNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverLongTransactionNoRplCheckNoSemiSync" || test == "ALL" {
		thistest.Name = "testSwitchoverLongTransactionNoRplCheckNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverLongTransactionNoRplCheckNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverLongTransactionNoRplCheckNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverLongTrxWithoutCommitNoRplCheckNoSemiSync" || test == "ALL" {
		thistest.Name = "testSwitchoverLongTrxWithoutCommitNoRplCheckNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverLongTrxWithoutCommitNoRplCheckNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverLongTrxWithoutCommitNoRplCheckNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverLongQueryNoRplCheckNoSemiSync" || test == "ALL" {
		thistest.Name = "testSwitchoverLongQueryNoRplCheckNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverLongQueryNoRplCheckNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverLongQueryNoRplCheckNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverNoReadOnlyNoRplCheck" || test == "ALL" {
		thistest.Name = "testSwitchoverNoReadOnlyNoRplCheck"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverNoReadOnlyNoRplCheck(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverNoReadOnlyNoRplCheck"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverReadOnlyNoRplCheck" || test == "ALL" {
		thistest.Name = "testSwitchoverReadOnlyNoRplCheck"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverReadOnlyNoRplCheck(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverReadOnlyNoRplCheck"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchover2TimesReplicationOkNoSemiSyncNoRplCheck" || test == "ALL" {
		thistest.Name = "testSwitchover2TimesReplicationOkNoSemiSyncNoRplCheck"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchover2TimesReplicationOkNoSemiSyncNoRplCheck(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchover2TimesReplicationOkNoSemiSyncNoRplCheck"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchover2TimesReplicationOkSemiSyncNoRplCheck" || test == "ALL" {
		thistest.Name = "testSwitchover2TimesReplicationOkSemiSyncNoRplCheck"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchover2TimesReplicationOkSemiSyncNoRplCheck(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchover2TimesReplicationOkSemiSyncNoRplCheck"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverBackPreferedMainNoRplCheckSemiSync" || test == "ALL" {
		thistest.Name = "testSwitchoverBackPreferedMainNoRplCheckSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverBackPreferedMainNoRplCheckSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverBackPreferedMainNoRplCheckSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverAllSubordinatesStopRplCheckNoSemiSync" || test == "ALL" {
		thistest.Name = "testSwitchoverAllSubordinatesStopRplCheckNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverAllSubordinatesStopRplCheckNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverAllSubordinatesStopRplCheckNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverAllSubordinatesStopNoSemiSyncNoRplCheck" || test == "ALL" {
		thistest.Name = "testSwitchoverAllSubordinatesStopNoSemiSyncNoRplCheck"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverAllSubordinatesStopNoSemiSyncNoRplCheck(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverAllSubordinatesStopNoSemiSyncNoRplCheck"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverAllSubordinatesDelayRplCheckNoSemiSync" || test == "ALL" {
		thistest.Name = "testSwitchoverAllSubordinatesDelayRplCheckNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverAllSubordinatesDelayRplCheckNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverAllSubordinatesDelayRplCheckNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSwitchoverAllSubordinatesDelayNoRplChecksNoSemiSync" || test == "ALL" {
		thistest.Name = "testSwitchoverAllSubordinatesDelayNoRplChecksNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSwitchoverAllSubordinatesDelayNoRplChecksNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSwitchoverAllSubordinatesDelayNoRplChecksNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSlaReplAllSubordinatesStopNoSemiSync" || test == "ALL" {
		thistest.Name = "testSlaReplAllSubordinatesStopNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSlaReplAllSubordinatesStopNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSlaReplAllSubordinatesStopNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testSlaReplAllSubordinatesDelayNoSemiSync" || test == "ALL" {
		thistest.Name = "testSlaReplAllSubordinatesDelayNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testSlaReplAllSubordinatesDelayNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testSlaReplAllSubordinatesDelayNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverNoRplChecksNoSemiSync" || test == "ALL" {
		thistest.Name = "testFailoverNoRplChecksNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverNoRplChecksNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverNoRplChecksNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverAllSubordinatesDelayNoRplChecksNoSemiSync" || test == "ALL" {
		if cl.InitTestCluster(conf, &thistest) == true {
			thistest.Name = "testFailoverAllSubordinatesDelayNoRplChecksNoSemiSync"
			res = testFailoverAllSubordinatesDelayNoRplChecksNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverAllSubordinatesDelayNoRplChecksNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverAllSubordinatesDelayRplChecksNoSemiSync" || test == "ALL" {
		thistest.Name = "testFailoverAllSubordinatesDelayRplChecksNoSemiSync"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverAllSubordinatesDelayRplChecksNoSemiSync(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverAllSubordinatesDelayRplChecksNoSemiSync"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverNumberFailureLimitReach" || test == "ALL" {
		thistest.Name = "testFailoverNumberFailureLimitReach"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverNumberFailureLimitReach(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverNumberFailureLimitReach"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	if test == "testFailoverTimeNotReach" || test == "ALL" {
		thistest.Name = "testFailoverTimeNotReach"
		if cl.InitTestCluster(conf, &thistest) == true {
			res = testFailoverTimeNotReach(cl, conf, &thistest)
			thistest.Result = regtest.getTestResultLabel(res)
		} else {
			thistest.Result = "ERR"
		}
		allTests["testFailoverTimeNotReach"] = thistest
		cl.CloseTestCluster(conf, &thistest)
	}
	vals := make([]cluster.Test, 0, len(allTests))
	keys := make([]string, 0, len(allTests))
	for key, val := range allTests {
		keys = append(keys, key)
		vals = append(vals, val)
	}
	sort.Strings(keys)
	for _, v := range keys {
		cl.LogPrintf("TEST", "Result %s -> %s", strings.Trim(v+strings.Repeat(" ", 60-len(v)), "test"), allTests[v].Result)
	}
	cl.CleanAll = false
	return vals
}

func (regtest *RegTest) getTestResultLabel(res bool) string {
	if res == false {
		return "FAIL"
	} else {
		return "PASS"
	}
}
