#include <sys/socket.h>
#include <sys/un.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <err.h>

int main() {
	struct sockaddr_un addr;
	memset(&addr, 0, sizeof(addr));
	addr.sun_family = AF_UNIX;
	strncpy(addr.sun_path, "/tmp/unix.test", sizeof(addr.sun_path)-1);

	int fd = socket(AF_UNIX, SOCK_STREAM, 0);
	if (fd < 0) {
		err(1, "socket");
	}
	if (connect(fd, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
		err(1, "bind");
	}
	write(fd, "I am c\n", 7);
	char buf[256];
	int n = read(fd, buf, sizeof(buf));
	printf("Read[%d] %.*s\n", n, n, buf);
	exit(0);
}
