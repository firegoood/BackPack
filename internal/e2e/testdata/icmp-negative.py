"""Raw packet negatives in disposable namespaces. No arbitrary remote targets."""
import base64, json, os, socket, struct, sys, time

mode, path = sys.argv[1:]
def checksum(b):
    if len(b)%2: b+=b'\0'
    s=sum(struct.unpack('!%dH'%(len(b)//2),b)); s=(s&65535)+(s>>16); s=(s&65535)+(s>>16)
    return (~s)&65535
if mode=='capture':
    with socket.socket(socket.AF_INET,socket.SOCK_RAW,socket.IPPROTO_ICMP) as s:
        s.settimeout(10)
        while True:
            b,addr=s.recvfrom(2048); offset=(b[0]&15)*4; icmp=b[offset:]
            if addr[0]=='10.66.0.2' and icmp[8:12]==b'FWIC' and len(icmp)>80 and icmp[31]==3:
                with open(path,'w') as f: json.dump({'packet':base64.b64encode(icmp).decode()},f)
                print('Captured authenticated native packet for replay/corruption negatives'); break
elif mode=='flood':
    packet=base64.b64decode(json.load(open(path))['packet'])
    sent=0; start=time.monotonic()
    with socket.socket(socket.AF_INET,socket.SOCK_RAW,socket.IPPROTO_ICMP) as s:
        s.setsockopt(socket.IPPROTO_IP,socket.IP_HDRINCL,1)
        for n in range(12000):
            icmp=bytearray(packet)
            # Keep a legitimate ICMP identifier and public demux tag, then test
            # replay, AEAD corruption, wrong SID, bad framing, and wrong peer IP.
            if n%5 in (1,2):
                icmp[-min(8,len(icmp)-8)]^=1
                pn=struct.unpack('!Q',icmp[40:48])[0]
                icmp[40:48]=struct.pack('!Q',(pn&(1<<63))|((1<<24)+n))
            elif n%5==3: icmp[32:]=os.urandom(len(icmp)-32)
            icmp[2:4]=b'\0\0'; icmp[2:4]=struct.pack('!H',checksum(bytes(icmp)))
            source='10.66.0.3' if n%5==4 else '10.66.0.2'
            h=struct.pack('!BBHHHBBH4s4s',0x45,0,20+len(icmp),n&65535,0x4000,64,1,0,socket.inet_aton(source),socket.inet_aton('10.66.0.1'))
            h=h[:10]+struct.pack('!H',checksum(h))+h[12:]
            s.sendto(h+icmp,('10.66.0.1',0)); sent+=1
            if n%30==0: time.sleep(.005)
    print(json.dumps({'negative_packets_sent':sent,'seconds':time.monotonic()-start}))
else: raise ValueError('test-only mode required')
