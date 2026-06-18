#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdarg.h>
#include <stdio.h>
#include <sys/ioctl.h>
#include <linux/fs.h>

typedef int (*orig_ioctl_fn)(int fd, unsigned long request, ...);

static orig_ioctl_fn real_ioctl = NULL;

int ioctl(int fd, unsigned long request, ...) {
    va_list args;
    void *argp;

    va_start(args, request);
    argp = va_arg(args, void *);
    va_end(args);

    if (request == BLKROGET) {
        fprintf(stderr, "blkro_override: intercepted BLKROGET on fd %d, returning writable\n", fd);
        if (argp) {
            *(int *)argp = 0;
        }
        return 0;
    }

    if (!real_ioctl) {
        real_ioctl = (orig_ioctl_fn)dlsym(RTLD_NEXT, "ioctl");
    }
    return real_ioctl(fd, request, argp);
}
