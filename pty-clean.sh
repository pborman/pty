#!/bin/ksh
# This script removes sessions in ~/.pty whose shell has exited.

cd $HOME/.pty

dead() {
	echo $1: dead
}
alive() {
	echo $1: alive
}

case $1 in
-r)
	dead() {
		rm -rf $1
	}
	alive() {
		echo keeping $1
	}
;;
-n) ;;
*) cat <<! 1>&2
usage: pty-clean [-r] [-n]
    -r  remove old sessions
    -n  report old sessions (do nothing)
!
exit 1
;;
esac

for i in @*; do
	if [ -e $i/pid ]; then
		pid=$(<$i/pid)
		if kill -0 $pid 2> /dev/null; then
			alive $i
		else
			dead $i
		fi
	else
		dead $i
	fi
done
