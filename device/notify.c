/* SDK toast guidance. Only validated four-digit pairing codes are dynamic. */
#include "ai_audio.h"
#include <string.h>
#include <stdio.h>
/* Documented SDK API, exported by the pinned library but missing in its header. */
extern int ai_audio_play_toast_text(const char *text);
int main(int argc,char **argv){
 if(argc==3 && (strcmp(argv[1],"binding-code")==0 || strcmp(argv[1],"binding-code-renewed")==0 || strcmp(argv[1],"binding-code-reminder")==0)){
  if(strlen(argv[2])!=4)return 2;
  const char *digits[]={"零","一","二","三","四","五","六","七","八","九"};
  for(int i=0;i<4;i++)if(argv[2][i]<'0'||argv[2][i]>'9')return 2;
  char text[512];
  if(strcmp(argv[1],"binding-code-reminder")==0)
   snprintf(text,sizeof(text),"当前验证码是，%s，%s，%s，%s。请在网页输入验证码完成绑定。",
    digits[argv[2][0]-'0'],digits[argv[2][1]-'0'],digits[argv[2][2]-'0'],digits[argv[2][3]-'0']);
  else snprintf(text,sizeof(text),"%s，%s，%s，%s，%s。验证码五分钟内有效，请登录会议网页，打开绑定眼镜，输入验证码。绑定后将自动开始会议记录。",
   strcmp(argv[1],"binding-code-renewed")==0?"原验证码已过期，新的验证码是":"验证码是",
   digits[argv[2][0]-'0'],digits[argv[2][1]-'0'],digits[argv[2][2]-'0'],digits[argv[2][3]-'0']);
  return ai_audio_play_toast_text(text)==AI_AUDIO_SUCCESS?0:1;
 }
 if(argc!=2)return 2;
 if(strcmp(argv[1],"bind-required")==0)
  return ai_audio_play_toast_text("请打开会议记录，听取验证码后在网页绑定眼镜") == AI_AUDIO_SUCCESS ? 0 : 1;
 if(strcmp(argv[1],"confirm-binding")==0)
  return ai_audio_play_toast_text("请在网页绑定眼镜，输入眼镜播报的验证码") == AI_AUDIO_SUCCESS ? 0 : 1;
 const char *text=0;
 if(strcmp(argv[1],"recording-started")==0)text="已开始会议记录";
 if(strcmp(argv[1],"binding-recording-started")==0)text="绑定成功，已开始会议记录";
 if(strcmp(argv[1],"start-failed")==0)text="会议记录未能启动，请检查网络和设备状态后重试";
 if(strcmp(argv[1],"scan-start")==0)text="请打开系统里的绑定二维码，我正在扫码，请正对屏幕保持片刻";
 if(strcmp(argv[1],"scan-recognized")==0)text="二维码已识别，请在网页核对眼镜信息，并点击确认绑定";
 if(strcmp(argv[1],"scan-adjust")==0)text="还没有识别到二维码，请正对二维码，调整距离和屏幕亮度";
 if(strcmp(argv[1],"binding-success")==0)text="会议账号绑定成功";
 if(strcmp(argv[1],"scan-failed")==0)text="扫码未成功，请重新打开绑定二维码，再说绑定会议账号";
 if(strcmp(argv[1],"qr-expired")==0)text="绑定二维码无效或已过期，请在系统里重新生成二维码";
 if(strcmp(argv[1],"rebind-required")==0)text="旧绑定凭证不可用，请先在原账号解绑旧设备，再打开会议记录";
 if(strcmp(argv[1],"network-failed")==0)text="无法连接会议服务器，请检查网络，录音尚未开始";
 if(strcmp(argv[1],"camera-failed")==0)text="相机抓拍失败，请检查眼镜相机后重新扫码";
 if(strcmp(argv[1],"confirm-timeout")==0)text="等待绑定已超时，请重新打开会议记录，再在网页输入验证码";
 if(text)return ai_audio_play_toast_text(text)==AI_AUDIO_SUCCESS?0:1;
 return 2;
}
