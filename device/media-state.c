/* Read-only SDK GPIO observer plus firmware playback-state query.
 * Does not own GPIO/audio/camera or change ai-core's physical actions. */
#include "ai_gpio.h"
#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <sys/un.h>
#include <time.h>
#include <unistd.h>
#include <signal.h>
static volatile sig_atomic_t running=1;
static void finish(int signal_number){(void)signal_number;running=0;}
static unsigned long long now_us(void){struct timespec t;clock_gettime(CLOCK_MONOTONIC,&t);return (unsigned long long)t.tv_sec*1000000+t.tv_nsec/1000;}
static int playback(void){
 int fd=socket(AF_UNIX,SOCK_STREAM,0);if(fd<0)return -1;
 struct timeval timeout={0,500000};setsockopt(fd,SOL_SOCKET,SO_RCVTIMEO,&timeout,sizeof(timeout));setsockopt(fd,SOL_SOCKET,SO_SNDTIMEO,&timeout,sizeof(timeout));
 struct sockaddr_un addr={0};addr.sun_family=AF_UNIX;strncpy(addr.sun_path,"/tmp/ai-core_audio_ctrl",sizeof(addr.sun_path)-1);
 char response[128];int result=-1,a,b,c;
 if(connect(fd,(struct sockaddr *)&addr,sizeof(addr))==0 && send(fd,"PLAYBACK_STATUS",15,MSG_NOSIGNAL)==15){
  int n=recv(fd,response,sizeof(response)-1,0);if(n>0){response[n]=0;if(sscanf(response,"OK|%d|%d|%d",&a,&b,&c)==3)result=a||b||c;}
 }
 close(fd);return result;
}
int main(int argc,char **argv){
 signal(SIGTERM,finish);signal(SIGINT,finish);
 int once=argc==2 && !strcmp(argv[1],"--once");
 gpio_event_hub_client_t hub;
 if(ai_gpio_hub_client_create(&hub)!=0 || ai_gpio_hub_client_connect(&hub)!=0)return 3;
 int gpios[GPIO_HUB_MAX_GPIO];int count=ai_gpio_hub_client_get_active_gpios(&hub,gpios,GPIO_HUB_MAX_GPIO);if(count<1)return 4;
 int last_pressed=-1,last_playing=-1;unsigned long long last=0;
 do{
  int pressed=0,healthy=ai_gpio_hub_client_is_service_alive(&hub);
  for(int i=0;i<count;i++){int state=ai_gpio_hub_client_get_gpio_state(&hub,gpios[i]);if(state<0)healthy=0;else if(state)pressed=1;}
  int playing=playback();if(playing<0)healthy=0;
  unsigned long long tick=now_us();
  if(once || !healthy || pressed!=last_pressed || playing!=last_playing || tick-last>=250000){
   printf("{\"atUs\":%llu,\"pressed\":%s,\"playing\":%s,\"healthy\":%s}\n",tick,pressed?"true":"false",playing>0?"true":"false",healthy?"true":"false");fflush(stdout);
   last_pressed=pressed;last_playing=playing;last=tick;
  }
  if(once)break;
  usleep(100000);
 }while(running);
 ai_gpio_hub_client_disconnect(&hub);ai_gpio_hub_client_destroy(&hub);return 0;
}
