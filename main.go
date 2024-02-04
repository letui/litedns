package main

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/miekg/dns"
	"log"
	"os"
	"strings"
	"time"
)

var dbc *sql.DB

func main() {

	refreshConnection()
	if len(os.Args) == 1 {
		startServer()
	}
	if len(os.Args) == 3 && os.Args[1] == "-p" {
		startServer()
	}

	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println(`
Usage: litedns [command] [arguments]

Without commands to run 'litedns' means start dns-server.

Commands:
	init                  Initialize the DNS database
	list                  List all DNS records
	add [type] [domain] [value]   Add a DNS record
	rm [type] [domain]           Remove a DNS record
	get [type] [domain]          Get the value of a specific DNS record

Options:
  -h, --help            Show this help message and exit
			`)
	}

	if len(os.Args) == 2 && os.Args[1] == "init" {
		initTables()
	}

	if len(os.Args) == 3 && os.Args[1] == "domain" {
		if os.Args[2] == "list" {
			listDomains("", 0, 50)
		}
	}

	if len(os.Args) == 5 {
		if strings.ToUpper(os.Args[1]) == "ADD" {
			addRecordToDB(os.Args[2], os.Args[3], os.Args[4])
		}
	}

	if len(os.Args) == 4 {
		if strings.ToUpper(os.Args[1]) == "GET" {
			getRecordFromDB(os.Args[2], os.Args[3])
		}
	}

	if len(os.Args) == 4 {
		if strings.ToUpper(os.Args[1]) == "RM" {
			rmRecordFromDB(os.Args[2], os.Args[3])
		}
	}

	defer dbc.Close()

}

func startServer() {
	// 创建 DNS 服务器实例

	var lstPort = ":53"
	if os.Args[1] == "-p" {
		lstPort = ":" + os.Args[2]
	}

	dnsServer := dns.Server{
		Addr:    lstPort, // 监听端口，可以根据需要更改
		Net:     "udp",
		Handler: dns.HandlerFunc(handleDNSRequest),
	}

	// 启动 DNS 服务器
	fmt.Println("DNS server started, listening on " + lstPort)
	err := dnsServer.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	defer w.Close()

	// 解析 DNS 请求中的查询信息
	query := r.Question[0]
	domain := query.Name
	recordType := query.Qtype

	// 查询数据库获取记录信息
	records, err := queryRecordsFromDB(domain, recordType)
	if err != nil {
		log.Println("Failed to query records from database:", err)
		return
	}

	// 构造并发送 DNS 响应
	response := buildDNSResponse(r, records)
	err = w.WriteMsg(response)
	if err != nil {
		log.Println("Failed to send DNS response:", err)
	}
}

func listDomains(name string, start, pageSize int) {
	refreshConnection()
	query, err := dbc.Query(`select * from domains where name like concat('%',?) limit ? offset ?`, name, pageSize, start)
	if err != nil {
		log.Println("Failed to query records from database:", err)
	}
	for query.Next() {
		var id, name, createdAt string
		query.Scan(&id, &name, &createdAt)
		fmt.Println(id, "\t", name, "\t", createdAt)
	}
	query.Close()
}

func getRecordFromDB(rqType string, keyname string) {
	refreshConnection()
	rows, err2 := dbc.Query("select id from record_types where name = ? ", strings.ToUpper(rqType))
	if err2 != nil {
		log.Println("Failed to query records from database:", err2)
	}
	var rqId int
	if rows.Next() {
		rows.Scan(&rqId)
	} else {
		return
	}
	rows.Close()

	query, err := dbc.Query(`select * from dns_records where name =? and record_type_id=?`, keyname, rqId)
	if err != nil {
		log.Println("Failed to query records from database:", err)
	}
	for query.Next() {
		var id, domainId, recordTypeId, name, value, ttl, createdAt string
		query.Scan(&id, &domainId, &recordTypeId, &name, &value, &ttl, &createdAt)
		fmt.Println(id, "\t", strings.ToUpper(rqType), "\t", name, "\t", value, "\t", ttl, "\t", createdAt)
	}
	query.Close()
}

func addRecordToDB(recordType string, name string, value string) {
	rows, err := dbc.Query("select id from record_types where name = ?", strings.ToUpper(recordType))
	if err != nil {
		fmt.Println("This recordType is not supported")
		return
	}
	rows.Next()
	var recordTypeId int
	rows.Scan(&recordTypeId)
	rows.Close()

	rows, err = dbc.Query("select count(*) from dns_records where name= ? and value = ? and record_type_id= ?", name, value, recordTypeId)
	if err != nil {
		fmt.Println("this error is not expected,sorry")
		return
	}
	var count int
	rows.Next()
	rows.Scan(&count)
	rows.Close()

	if count > 0 {
		fmt.Println("This record has already exist")
		return
	} else {
		if strings.ToUpper(recordType) == "A" || strings.ToUpper(recordType) == "AAAA" || strings.ToUpper(recordType) == "CNAME" {
			rows, err = dbc.Query("select * from domains where name = ?", name+".")
			if err != nil {
				fmt.Println("This record has already exist")
				return
			}
			var domainId int
			if rows.Next() {
				rows.Scan(&domainId)
				rows.Close()
				dbc.Exec("insert into dns_records (domain_id,record_type_id,name,value) values (?,?,?,?)", domainId, recordTypeId, name, value)

			} else {
				rows.Close()
				// 执行插入语句
				result, err := dbc.Exec("insert into domains (name) values (?)", name+".")
				if err != nil {
					panic(err)
				}
				// 获取插入后生成的主键
				domainId, err := result.LastInsertId()
				if err != nil {
					panic(err)
				}
				dbc.Exec("insert into dns_records (domain_id,record_type_id,name,value) values (?,?,?,?)", domainId, recordTypeId, name, value)
			}
			fmt.Println("Operation is completed")
		}
	}
}

func rmRecordFromDB(recordType string, name string) {
	rows, err := dbc.Query("select id from record_types where name = ?", strings.ToUpper(recordType))
	if err != nil {
		fmt.Println("This recordType is not supported")
		return
	}
	rows.Next()
	var recordTypeId int
	rows.Scan(&recordTypeId)
	rows.Close()

	exec, err := dbc.Exec("delete from dns_records where record_type_id=? and name=? ", recordTypeId, name)

	if err != nil {
		fmt.Println("sorry,something got wrong")
	}
	affected, _ := exec.RowsAffected()
	if affected > 0 {
		fmt.Println("Operation is completed")
	} else {
		fmt.Println("Target is not exist")
	}
}

func queryRecordsFromDB(domain string, recordType uint16) ([]dns.RR, error) {
	refreshConnection()
	// 根据域名和记录类型查询数据库
	rows, err := dbc.Query("select r.name,r.value,r.ttl,t.name from dns_records r join record_types t on r.record_type_id = t.id join domains d on r.domain_id = d.id where d.name = ? AND t.id = ? ;", domain, recordType)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	defer rows.Close()

	// 解析数据库结果并构造 DNS 记录
	var records []dns.RR
	for rows.Next() {
		var name, value string
		var ttl int
		var recordTypeName string
		err := rows.Scan(&name, &value, &ttl, &recordTypeName)
		if err != nil {
			return nil, err
		}

		recordType, ok := dns.StringToType[recordTypeName]
		if !ok {
			log.Println("Unsupported record type:", recordTypeName, " with ", recordType)
			continue
		}

		rr, err := dns.NewRR(fmt.Sprintf("%s %d IN %s %s", name, ttl, recordTypeName, value))
		if err != nil {
			log.Println("Failed to create DNS record:", err)
			continue
		}
		records = append(records, rr)
	}

	return records, nil
}

func buildDNSResponse(request *dns.Msg, records []dns.RR) *dns.Msg {
	response := new(dns.Msg)
	response.SetReply(request)

	// 添加查询结果到响应中
	response.Answer = records

	// 设置响应头部信息
	response.Authoritative = true
	response.RecursionAvailable = true
	response.MsgHdr.RecursionDesired = true
	response.MsgHdr.Id = request.MsgHdr.Id

	return response
}

func refreshConnection() {
	var err error = nil
	if dbc == nil {
		dbc, err = sql.Open("sqlite3", "dns.db")
		if err != nil {
			log.Fatal(err, dbc)
		}
		dbc.SetConnMaxLifetime(time.Hour)
	}
}

func initTables() error {
	var err error
	sqlquerys := `
-- 建立域名表
CREATE TABLE domains (
                         id INTEGER PRIMARY KEY AUTOINCREMENT,
                         name TEXT NOT NULL,
                         created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 建立 DNS 记录类型表
CREATE TABLE record_types (
                              id INTEGER PRIMARY KEY,
                              name TEXT NOT NULL
);

-- 插入 DNS 记录类型数据
INSERT INTO record_types (id, name) VALUES
                                        (0, 'None'),
(1, 'A'),
(2, 'NS'),
(3, 'MD'),
(4, 'MF'),
(5, 'CNAME'),
(6, 'SOA'),
(7, 'MB'),
(8, 'MG'),
(9, 'MR'),
(10, 'NULL'),
(12, 'PTR'),
(13, 'HINFO'),
(14, 'MINFO'),
(15, 'MX'),
(16, 'TXT'),
(17, 'RP'),
(18, 'AFSDB'),
(19, 'X25'),
(20, 'ISDN'),
(21, 'RT'),
(23, 'NSAPPTR'),
(24, 'SIG'),
(25, 'KEY'),
(26, 'PX'),
(27, 'GPOS'),
(28, 'AAAA'),
(29, 'LOC'),
(30, 'NXT'),
(31, 'EID'),
(32, 'NIMLOC'),
(33, 'SRV'),
(34, 'ATMA'),
(35, 'NAPTR'),
(36, 'KX'),
(37, 'CERT'),
(39, 'DNAME'),
(41, 'OPT'),
(42, 'APL'),
(43, 'DS'),
(44, 'SSHFP'),
(45, 'IPSECKEY'),
(46, 'RRSIG'),
(47, 'NSEC'),
(48, 'DNSKEY'),
(49, 'DHCID'),
(50, 'NSEC3'),
(51, 'NSEC3PARAM'),
(52, 'TLSA'),
(53, 'SMIMEA'),
(55, 'HIP'),
(56, 'NINFO'),
(57, 'RKEY'),
(58, 'TALINK'),
(59, 'CDS'),
(60, 'CDNSKEY'),
(61, 'OPENPGPKEY'),
(62, 'CSYNC'),
(63, 'ZONEMD'),
(64, 'SVCB'),
(65, 'HTTPS'),
(99, 'SPF'),
(100, 'UINFO'),
(101, 'UID'),
(102, 'GID'),
(103, 'UNSPEC'),
(104, 'NID'),
(105, 'L32'),
(106, 'L64'),
(107, 'LP'),
(108, 'EUI48'),
(109, 'EUI64'),
(256, 'URI'),
(257, 'CAA'),
(258, 'AVC'),
(260, 'AMTRELAY'),
(249, 'TKEY'),
(250, 'TSIG'),
(251, 'IXFR'),
(252, 'AXFR'),
(253, 'MAILB'),
(254, 'MAILA'),
(255, 'ANY'),
(32768, 'TA'),
(32769, 'DLV'),
(65535, 'Reserved');

-- 建立 DNS 记录表
CREATE TABLE dns_records (
                             id INTEGER PRIMARY KEY AUTOINCREMENT,
                             domain_id INT NOT NULL,
                             record_type_id INT NOT NULL,
                             name TEXT NOT NULL,
                             value TEXT NOT NULL,
                             ttl INT DEFAULT 3600,
                             created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                             FOREIGN KEY (domain_id) REFERENCES domains(id),
                             FOREIGN KEY (record_type_id) REFERENCES record_types(id)
);`

	_, err = dbc.Exec(sqlquerys)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Database is initialized okay！")
	return err
}
