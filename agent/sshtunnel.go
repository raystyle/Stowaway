package agent

import (
	"Stowaway/node"
	"Stowaway/utils"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

//利用sshtunnel来连接下一个节点，以此在防火墙限制流量时仍然可以进行穿透
func SshTunnelNextNode(info string, nodeid string) error {
	var authpayload ssh.AuthMethod
	spiltedinfo := strings.Split(info, ":::")
	host := spiltedinfo[0]
	username := spiltedinfo[1]
	authway := spiltedinfo[2]
	lport := spiltedinfo[3]
	method := spiltedinfo[4]

	if method == "1" {
		authpayload = ssh.Password(authway)
	} else if method == "2" {
		key, err := ssh.ParsePrivateKey([]byte(authway))
		if err != nil {
			sshMess, _ := utils.ConstructPayload(utils.AdminId, "", "COMMAND", "SSHCERTERROR", " ", " ", 0, nodeid, AgentStatus.AESKey, false)
			ProxyChan.ProxyChanToUpperNode <- sshMess
			return err
		}
		authpayload = ssh.PublicKeys(key)
	}

	SshClient, err := ssh.Dial("tcp", host, &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{authpayload},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		sshMess, _ := utils.ConstructPayload(utils.AdminId, "", "COMMAND", "SSHTUNNELRESP", " ", "FAILED", 0, nodeid, AgentStatus.AESKey, false)
		ProxyChan.ProxyChanToUpperNode <- sshMess
		return err
	}

	nodeConn, err := SshClient.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", lport))

	if err != nil {
		sshMess, _ := utils.ConstructPayload(utils.AdminId, "", "COMMAND", "SSHTUNNELRESP", " ", "FAILED", 0, nodeid, AgentStatus.AESKey, false)
		ProxyChan.ProxyChanToUpperNode <- sshMess
		nodeConn.Close()
		return err
	}

	err = node.SendSecret(nodeConn, AgentStatus.AESKey)
	if err != nil {
		sshMess, _ := utils.ConstructPayload(utils.AdminId, "", "COMMAND", "SSHTUNNELRESP", " ", "FAILED", 0, nodeid, AgentStatus.AESKey, false)
		ProxyChan.ProxyChanToUpperNode <- sshMess
		nodeConn.Close()
		return err
	}

	helloMess, _ := utils.ConstructPayload(nodeid, "", "COMMAND", "STOWAWAYAGENT", " ", " ", 0, utils.AdminId, AgentStatus.AESKey, false)
	nodeConn.Write(helloMess)
	for {
		command, err := utils.ExtractPayload(nodeConn, AgentStatus.AESKey, utils.AdminId, true)
		if err != nil {
			sshMess, _ := utils.ConstructPayload(utils.AdminId, "", "COMMAND", "SSHTUNNELRESP", " ", "FAILED", 0, nodeid, AgentStatus.AESKey, false)
			ProxyChan.ProxyChanToUpperNode <- sshMess
			nodeConn.Close()
			return err
		}
		switch command.Command {
		case "INIT":
			NewNodeMessage, _ := utils.ConstructPayload(utils.AdminId, "", "COMMAND", "NEW", " ", nodeConn.RemoteAddr().String(), 0, nodeid, AgentStatus.AESKey, false)
			node.NodeInfo.LowerNode.Payload[utils.AdminId] = nodeConn
			node.NodeStuff.ControlConnForLowerNodeChan <- nodeConn
			node.NodeStuff.NewNodeMessageChan <- NewNodeMessage
			node.NodeStuff.IsAdmin <- false
			sshMess, _ := utils.ConstructPayload(utils.AdminId, "", "COMMAND", "SSHTUNNELRESP", " ", "SUCCESS", 0, nodeid, AgentStatus.AESKey, false)
			ProxyChan.ProxyChanToUpperNode <- sshMess
			return nil
		case "REONLINE":
			//普通节点重连
			node.NodeStuff.ReOnlineId <- command.CurrentId
			node.NodeStuff.ReOnlineConn <- nodeConn
			<-node.NodeStuff.PrepareForReOnlineNodeReady
			NewNodeMessage, _ := utils.ConstructPayload(nodeid, "", "COMMAND", "REONLINESUC", " ", " ", 0, nodeid, AgentStatus.AESKey, false)
			nodeConn.Write(NewNodeMessage)
			sshMess, _ := utils.ConstructPayload(utils.AdminId, "", "COMMAND", "SSHTUNNELRESP", " ", "SUCCESS", 0, nodeid, AgentStatus.AESKey, false)
			ProxyChan.ProxyChanToUpperNode <- sshMess
			return nil
		}
	}
}
