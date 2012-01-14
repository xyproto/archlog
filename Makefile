all:
	6g svnchangelog.go
	6l -o svnchangelog svnchangelog.6

gcc:
	gccgo svnchangelog.go -o svnchangelog

clean:
	rm -f *.6 svnchangelog
