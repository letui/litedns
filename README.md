Usage: litedns [command] [arguments]

1.Without commands to run 'litedns' means start dns-server.
2.You must complete the initialization process before starting the DNS server.

Commands:
	init			Initialize the DNS database
	dns types		List all types of DNS-Record
	domain list		List all domains
	add [type] [domain] [value]		Add a DNS record
	rm [type] [domain]			Remove a DNS record
	get [type] [domain]			Get the value of a specific DNS record

Options:
  -h, --help            Show this help message and exit
