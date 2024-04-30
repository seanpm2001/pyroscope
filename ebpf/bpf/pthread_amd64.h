//
// Created by korniltsev on 11/21/23.
//

#ifndef PYROEBPF_PTHREAD_AMD64_H
#define PYROEBPF_PTHREAD_AMD64_H

#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_core_read.h"
#include "pyoffsets.h"


#if !defined(__TARGET_ARCH_x86)
#error "Wrong architecture"
#endif

static int pthread_getspecific_musl(void *ctx, const struct libc *libc, int32_t key, void **out, const void *fsbase);
static int pthread_getspecific_glibc(void *ctx, const struct libc *libc, int32_t key, void **out, const void *fsbase);

static __always_inline int pyro_pthread_getspecific(void *ctx, struct libc *libc, int32_t key, void **out) {
    if (key == -1) {
        return -1;
    }
    struct task_struct *task = (struct task_struct *) bpf_get_current_task();
    if (task == NULL) {
        return -1;
    }
    void *tls_base = NULL;
    short unsigned int fsindex = 0;

    log_debug("pyro_pthread_getspecific(amd64) key=%d pthread_size=%llx o_pthread_specific1stblock=%llx", key, libc->pthread_size, libc->pthread_specific1stblock);
    if (bpf_core_read(&tls_base, sizeof(tls_base), &task->thread.fsbase)) {
        log_error("pyro_pthread_getspecific(amd64) failed to read fsbase");
        return -1;
    }
    if (bpf_core_read(&fsindex, sizeof(fsindex), &task->thread.fsindex)) {
        log_error("pyro_pthread_getspecific(amd64) failed to read fsindex");
        return -1;
    }
    log_debug("pyro_pthread_getspecific(amd64)  fsbase = 0x%llx fsindex = 0x%x musl=%d", tls_base, fsindex, libc->musl);


    if (libc->musl) {
        return pthread_getspecific_musl(ctx, libc, key, out, tls_base);

    }
    return pthread_getspecific_glibc(ctx, libc, key, out, tls_base);

}

static __always_inline int pthread_getspecific_glibc(void *ctx, const struct libc *libc, int32_t key, void **out, const void *fsbase) {
    void *tmp[2] = {NULL, NULL};
    if (key >= 32) {
        return -1; // it is possible to implement this branch, but it's not needed as autoTLSkey is almost always 0
    }
    void *thread_self = NULL;
    try_read(thread_self,  fsbase + 0x10);
    log_debug("pthread_getspecific_glibc(amd64) thread_self=%llx", thread_self);
    // This assumes autoTLSkey < 32, which means that the TLS is stored in
//   pthread->specific_1stblock[autoTLSkey]
    try_read(tmp, thread_self + libc->pthread_specific1stblock + key * 0x10)
    log_debug("pthread_getspecific_glibc(amd64) res=%llx %llx", tmp[0], tmp[1]);
    *out = tmp[1];
    return 0;
}

static __always_inline int pthread_getspecific_musl(void *ctx, const struct libc *libc, int32_t key, void **out,
                                    const void *fsbase) {
    // example from musl 1.2.4 from alpine 3.18
//        static void *__pthread_getspecific(pthread_key_t k)
//        {
//            struct pthread *self = __pthread_self();
//            return self->tsd[k];
//        }
//
//        #define __pthread_self() ((pthread_t)__get_tp())
//
//        static inline uintptr_t __get_tp()
//        {
//            uintptr_t tp;
//            __asm__ ("mov %%fs:0,%0" : "=r" (tp) );
//            return tp;
//        }
//
//00000000000563f7 <pthread_getspecific>:
//   563f7:       64 48 8b 04 25 00 00    mov    rax,QWORD PTR fs:0x0
//   563fe:       00 00
//   56400:       48 8b 80 80 00 00 00    mov    rax,QWORD PTR [rax+0x80]  ; << tsd
//   56407:       89 ff                   mov    edi,edi
//   56409:       48 8b 04 f8             mov    rax,QWORD PTR [rax+rdi*8]
//   5640d:       c3                      ret
    void *tmp = NULL;
    try_read(tmp, fsbase)
    log_debug("pthread_getspecific_musl(amd64) tmp=%llx", tmp);
    try_read(tmp, tmp + libc->pthread_specific1stblock)
    log_debug("pthread_getspecific_musl(amd64) tmp2=%llx", tmp);
    try_read(tmp, tmp + key * 0x8)
    log_debug("pthread_getspecific_musl(amd64) res=%llx", tmp);
    *out = tmp;
    return 0;
}

#endif //PYROEBPF_PTHREAD_AMD64_H
