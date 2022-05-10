IMAGE=rminnich/sourcery:latest
all: cat date cpud cpu elvish ls sourcery Dockerfile
	docker build . -t $(IMAGE)

sourcery:
	sudo rm -rf /tmp/docker docker
	(cd .. && ./sourcery -d /tmp/docker git@github.com:u-root/u-root git@github.com:u-root/cpu git@github.com:u-root/NiChrome)
	rsync -av /tmp/docker/ .

cpud:
	CGO_ENABLED=0 GOBIN=`pwd` go install  github.com/u-root/cpu/cmds/cpud@latest

cpu:
	CGO_ENABLED=0 GOBIN=`pwd` go install  github.com/u-root/cpu/cmds/cpu@latest

cat:
	CGO_ENABLED=0 GOBIN=`pwd` go install  github.com/u-root/u-root/cmds/core/cat

date:
	CGO_ENABLED=0 GOBIN=`pwd` go install  github.com/u-root/u-root/cmds/core/date

elvish:
	CGO_ENABLED=0 GOBIN=`pwd` go install  github.com/u-root/u-root/cmds/core/elvish@latest

ls:
	CGO_ENABLED=0 GOBIN=`pwd` go install  github.com/u-root/u-root/cmds/core/ls@latest

rundocker:
	-docker rm sourcery_test
	echo test if "$(KEY)" is empty, if so next line will fail.
	stat $(KEY)
	echo on start, run /linux_amd64/bin/init
	echo To exit, kill the init process.
	docker run --mount type=bind,source=$(KEY).pub,target=/key.pub --mount type=bind,source=$(KEY),target=/key --name sourcery_test --privileged=true -t -i -p 8000-8020:8000-8020 -p 23:23 $(IMAGE)

run:
	@echo ===================================================
	@echo Check the date, is it `date`
	cpu -key key localhost date
	@echo ===================================================
	@echo Note the pwd -- it is `pwd`
	cpu -key key localhost pwd
	@echo ===================================================
	@echo Note that ls is the same local or remote
	cpu -key key localhost ls
	@echo ===================================================
	@echo note that hostname is not `hostname`
	cpu -key key localhost hostname
	@echo ===================================================
	@echo note the mounts ...
	cpu -key key localhost mount | grep 127.0.0.1
	@echo ===================================================
	@echo run with no 9p and no mounts
	@echo note that there is no homedir, so you must set PWD=/
	PWD=/ cpu -key key -9p=false -namespace="" localhost /bin/date
	@echo you can also prune the namespace
	@echo note that there is no homedir, so you must set PWD=/
	PWD=/ cpu -key key -9p=false -namespace="/home" localhost /bin/cat /proc/mounts | grep 127.0.0.1


cpuindocker:
	docker exec -it cpud_test /bin/cpu -key /key localhost /bin/date

push:
	docker push
clean:
	sudo rm -rf dev docker etc go key key.pub linux_amd64 pkg src tmp
	rm -f cat cpu cpud date elvish ls
