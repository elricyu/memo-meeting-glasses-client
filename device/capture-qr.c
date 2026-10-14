/* RV1106B camera SDK IPC adapter. Uses ai-core's JPEG service; never owns VI/VENC.
 * The firmware adds AI_MEDIA_CAPTURE; the reference SDK gitlink still polls SHM.
 * Layout verified against the installed SDK's ai_core_capture (36-byte header).
 */
#include "ai_camera.h"
#include <fcntl.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <sys/un.h>
#include <unistd.h>

typedef struct {
 uint32_t magic, request, ready, size, width, height, format, sequence, error;
 unsigned char jpeg[];
} camera_shm;
_Static_assert(sizeof(camera_shm)==36,"Camera SDK ABI mismatch");

static int command(const char *request,const char *expected){
 int fd=socket(AF_UNIX,SOCK_STREAM,0);if(fd<0)return -1;
 struct timeval timeout={6,0};
 setsockopt(fd,SOL_SOCKET,SO_RCVTIMEO,&timeout,sizeof(timeout));
 setsockopt(fd,SOL_SOCKET,SO_SNDTIMEO,&timeout,sizeof(timeout));
 struct sockaddr_un address={0};address.sun_family=AF_UNIX;
 strncpy(address.sun_path,AI_MEDIA_CTRL_SOCKET_PATH,sizeof(address.sun_path)-1);
 char response[512];int result=-1;
 if(connect(fd,(struct sockaddr *)&address,sizeof(address))==0 &&
    send(fd,request,strlen(request),MSG_NOSIGNAL)==(ssize_t)strlen(request)){
  ssize_t count=recv(fd,response,sizeof(response)-1,0);
  if(count>0){response[count]=0;if(strncmp(response,expected,strlen(expected))==0)result=0;}
 }
 close(fd);return result;
}
int main(int argc,char **argv){
 if(argc!=2)return 2;
 if(command("AI_MEDIA_REQUEST","AI_MEDIA_SHM_READY")!=0){fprintf(stderr,"Camera service unavailable\n");return 3;}
 int result=4,fd=shm_open(AI_MEDIA_SHM_NAME,O_RDONLY,0);
 camera_shm *frame=MAP_FAILED;
 if(fd>=0){
  struct stat info;
  if(fstat(fd,&info)==0 && info.st_size>=AI_MEDIA_SHM_SIZE)
   frame=mmap(NULL,AI_MEDIA_SHM_SIZE,PROT_READ,MAP_SHARED,fd,0);
 }
 if(frame!=MAP_FAILED && frame->magic==0x414d4449 && command("AI_MEDIA_CAPTURE|5000","OK|")==0 &&
    frame->error==0 && frame->format==AI_MEDIA_FORMAT_JPEG && frame->size>2 && frame->size<=2097152 &&
    frame->jpeg[0]==0xff && frame->jpeg[1]==0xd8){
  int output=open(argv[1],O_WRONLY|O_CREAT|O_TRUNC|O_NOFOLLOW,0600);
  if(output>=0){
   fchmod(output,0600);size_t written=0,expected=frame->size;
   while(written<expected){ssize_t n=write(output,frame->jpeg+written,expected-written);if(n<=0)break;written+=(size_t)n;}
   int close_result=close(output);if(written==expected && close_result==0)result=0;
  }
 }
 if(frame!=MAP_FAILED)munmap(frame,AI_MEDIA_SHM_SIZE);
 if(fd>=0)close(fd);
 command("AI_MEDIA_RELEASE","AI_MEDIA_RELEASED");
 if(result)fprintf(stderr,"QR JPEG capture failed\n");
 return result;
}
