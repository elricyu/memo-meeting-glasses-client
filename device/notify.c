/* SDK-generated toast guidance; fixed business phrases only, no credentials. */
#include "ai_audio.h"
#include <string.h>
/* Documented SDK API, exported by the pinned library but missing in its header. */
extern int ai_audio_play_toast_text(const char *text);
int main(int argc,char **argv){
 if(argc!=2)return 2;
 if(strcmp(argv[1],"bind-required")==0)
  return ai_audio_play_toast_text("请先在网页生成二维码，用眼镜扫码绑定会议账号") == AI_AUDIO_SUCCESS ? 0 : 1;
 if(strcmp(argv[1],"confirm-binding")==0)
  return ai_audio_play_toast_text("请先在网页确认绑定，再打开会议记录") == AI_AUDIO_SUCCESS ? 0 : 1;
 const char *text=0;
 if(strcmp(argv[1],"scan-start")==0)text="请打开系统里的绑定二维码，我正在扫码，请正对屏幕保持片刻";
 if(strcmp(argv[1],"scan-recognized")==0)text="二维码已识别，请在网页核对眼镜信息，并点击确认绑定";
 if(strcmp(argv[1],"scan-adjust")==0)text="还没有识别到二维码，请正对二维码，调整距离和屏幕亮度";
 if(strcmp(argv[1],"binding-success")==0)text="会议账号绑定成功";
 if(strcmp(argv[1],"scan-failed")==0)text="扫码未成功，请重新打开绑定二维码，再说绑定会议账号";
 if(strcmp(argv[1],"qr-expired")==0)text="绑定二维码无效或已过期，请在系统里重新生成二维码";
 if(strcmp(argv[1],"rebind-required")==0)text="旧绑定凭证不可用，请先在原账号解绑旧设备，再打开二维码扫码";
 if(strcmp(argv[1],"network-failed")==0)text="无法连接会议服务器，请检查网络，录音尚未开始";
 if(strcmp(argv[1],"camera-failed")==0)text="相机抓拍失败，请检查眼镜相机后重新扫码";
 if(strcmp(argv[1],"confirm-timeout")==0)text="等待确认已超时，请在系统里查看绑定状态后重试";
 if(text)return ai_audio_play_toast_text(text)==AI_AUDIO_SUCCESS?0:1;
 return 2;
}
