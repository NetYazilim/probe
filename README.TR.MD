# Probe - Çok Protokollü Health Check

[English README](README.md)

Go ile yazılmış çok protokollü sağlık kontrolü kütüphanesi. Öncelikle projelerinize paket olarak import edin; basit kontroller için beraberindeki CLI aracını kullanın.

> **Not:** Bu depodaki CLI aracı, temel olarak kütüphanenin yeteneklerini göstermek ve basit kontroller yapmak için tasarlanmıştır. Bu kütüphaneyi temel alan tam kapsamlı ayrıntılı log aracı için lütfen **[NetYazilim/izle](https://github.com/NetYazilim/izle)** projesini inceleyin.

## Özellikler

- **ICMP Ping Probe** - ICMP Echo kullanarak ana bilgisayar erişilebilirliğini kontrol edin (IP adresleri ve hostname'leri destekler; Linux'ta IPv6 desteği vardır).
- **TCP Probe** - Port bağlantısı ve bağlantı bilgilerini doğrulayın (IPv4 ve IPv6 hedeflerde çalışır).
- **HTTP/HTTPS Probe** - Web servisi sağlığını durum kodu kontrolleriyle doğrulayın (IPv4/IPv6 uç noktaları otomatik desteklenir).
- **TLS/SSL Probe** - IPv4 ve IPv6 üzerinde iki modu destekler: strict TLS doğrulaması (`tls`) ve yalnızca sertifika inceleme modu (`tls-cert`). Sertifika-only modunda, sunucu mTLS/istemci sertifikası istese ve tam TLS oturumu kurulmasa bile sunulan sunucu sertifikası okunabilir. Her iki mod da geçerlilik aralığını kontrol eder ve sunulan zincirin tamamını raporlar; böylece leaf'ten önce dolan bir ara sertifika gözden kaçmaz.
- **Başarı eşiği** - denemelerin kaçının cevap vermesi gerektiğine siz karar verirsiniz. Probe, eşiğe ulaşır ulaşmaz durur; kalan denemeler eşiğe artık yetişemiyorsa yine durur.
- **İptal edilebilir** - her probe bir `context.Context` alır ve onu yalnızca denemeler arasında değil, bağlanırken, handshake yaparken ve cevap beklerken de dinler.
- **Ayarlanabilir ICMP payload boyutu** - `-size` ping'i path MTU kontrolü için kullanılabilir kılar; ayrıca farklı hedeflere aynı boyutta paket gönderildiği için gidiş-dönüş süreleri karşılaştırılabilir olur.
- **Asgari bağımlılık** - `golang.org/x/net` yalnızca Linux'ta ICMP ping için gereklidir; TCP, HTTP ve TLS probe'ları ile Windows ve macOS derlemeleri tek başına Go standart kütüphanesini kullanır.

## Platform Desteği

- **Linux** - UDP tabanlı ICMP kullanır (unprivileged ping için kernel desteği gerekir) ve IPv4 ile IPv6 hedeflerini destekler.
- **Windows** - Win32 ICMP API kullanır (özel yetki gerekmez) ve şu anda ICMP ping için yalnızca IPv4 desteklenir.
- **macOS ve diğer platformlar** - ICMP ping uygulanmamıştır ve açık bir hata döndürür; TCP, HTTP ve TLS probe'ları normal çalışır.

### Linux ICMP Ping Konfigürasyonu

Linux'ta, çekirdek unprivileged ICMP ping'e izin vermelidir. Kontrol etmek için:

```bash
cat /proc/sys/net/ipv4/ping_group_range
```

**Çıktı anlamları:**
- `1 0` - ICMP ping unprivileged kullanıcılar için kapalı
- `0 2147483647` - Tüm kullanıcılar root olmadan ICMP ping yapabilir

**Unprivileged ICMP ping'i etkinleştirmek için:**

```bash
sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"
```

> systemd tabanlı bir dağıtımda bu genellikle zaten varsayılandır: systemd,
> `sysctl.d/50-default.conf` dosyasında *"ping(8) without CAP_NET_ADMIN and
> CAP_NET_RAW"* yorumuyla `net.ipv4.ping_group_range = 0 2147483647` satırını
> getirir. Bu komuta asıl ihtiyaç duyacağınız yer, `systemd-sysctl`
> çalıştırmayan container'lar ve minimal imajlardır; oralarda kernel'in
> varsayılanı olan `1 0` (kapalı) geçerli kalır. Varsaymak yerine önce kontrol
> edin.

**Değişikliği kalıcı hale getirmek için:**

```bash
echo "net.ipv4.ping_group_range = 0 2147483647" | sudo tee /etc/sysctl.d/99-ping.conf
sudo sysctl -p /etc/sysctl.d/99-ping.conf
```

## Kurulum

```bash
go get "github.com/netyazilim/probe"
```

## Kütüphane olarak kullanım

Her probe kendi paketinde ve aynı iki giriş noktasını sunuyor: yeni kod için
`RunContext`, eski API'ye göre yazılmış çağrılar için `Run`.

```go
import (
    "context"
    "time"

    "github.com/netyazilim/probe"
    "github.com/netyazilim/probe/pkg/probeping"
    "github.com/netyazilim/probe/pkg/probetcp"
    "github.com/netyazilim/probe/pkg/probehttp"
    "github.com/netyazilim/probe/pkg/probetls"
)

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

r := probeping.RunContext(ctx, "8.8.8.8", probe.Options{
    MaxAttempts:      5,
    SuccessThreshold: 3,               // 5 denemenin 3'ü cevap vermeli
    Timeout:          time.Second,     // deneme başına
    Interval:         200 * time.Millisecond,
    Size:             1472,            // ICMP payload baytı
    Logger:           logger,          // *slog.Logger, nil olabilir
})

if r.Success {
    fmt.Println(r.Duration, r.Min, r.Avg, r.Max)
}
```

### Options

Sıfır değeri kullanılabilir: sıfır bırakılan her alan varsayılanını alır.

| Alan | Varsayılan | Anlamı |
|---|---|---|
| `MaxAttempts` | 3 | Deneme üst sınırı; hedef değil |
| `SuccessThreshold` | 1 | Geçmek için gereken başarılı deneme; `MaxAttempts`'i aşamaz |
| `Timeout` | 1s (TLS'te 5s) | Tek bir denemeyi sınırlar, tüm çalışmayı değil |
| `Interval` | 500ms | Denemeler arası bekleme |
| `Size` | 56 | ICMP payload baytı; diğer probe'lar yok sayar |
| `Logger` | yok sayar | Deneme ayrıntısını alan `*slog.Logger` |

### Result

Her `Result`, `probe.Summary`'yi gömer; böylece bu alanlar hangi probe
üretirse üretsin aynı anlama gelir ve aynı şekilde okunur:

| Alan | Anlamı |
|---|---|
| `Success` | `SuccessThreshold`'a ulaşıldı mı |
| `Attempts`, `Successes`, `Failures` | Kaç deneme yapıldı ve nasıl sonuçlandı |
| `Duration` | En son başarılı deneme |
| `Min`, `Avg`, `Max` | Başarılı denemeler üzerinden; tek başarı varsa üçü de `Duration`'a eşittir |
| `Error` | Görülen son hata; `Success` true ise `nil` |

Her paket bunun üstüne kendi alanlarını ekler: ping için
`Address`/`ResolvedIP`/`BytesRecv`, tcp için
`Host`/`Port`/`LocalAddr`/`RemoteAddr`, http için `URL`/`StatusCode`, tls için
sertifika ayrıntıları.

### Geriye uyumluluk

`Run(target, maxAttempts, timeout)` her pakette duruyor ve eskisi gibi
davranıyor; mevcut çağrı satırlarınız değişmeden derlenir. Yalnızca import
yolları değişti: `pkg/http` ve `pkg/tls` standart kütüphaneyi gölgeleyip her
çağıran tarafta alias'a zorluyordu, bu yüzden paketler artık `pkg/probeping`,
`pkg/probetcp`, `pkg/probehttp` ve `pkg/probetls`.

## Kullanım

```bash
# Yardım görüntüle
./probe

# ICMP Ping (IP Adresleri veya Hostname'ler)
./probe ping 8.8.8.8
./probe ping google.com
./probe ping -attempts 5 8.8.8.8
./probe ping -attempts 5 -threshold 3 8.8.8.8     # 5 denemenin 3'ü yeterli
./probe ping -size 1472 gateway.local             # path MTU kontrolü
./probe ping -timeout 2s google.com
./probe ping -debug 8.8.8.8                       # deneme ayrıntısı stderr'e
./probe ping -loop 5s 8.8.8.8                     # Her 5 saniyede çalıştır

# TCP Probe (Port Bağlantısı)
./probe tcp example.com:22
./probe tcp google.com:443
./probe tcp -attempts 5 google.com:443
./probe tcp -loop 10s google.com:443             # Her 10 saniyede çalıştır

# HTTP/HTTPS (URL'ler)
./probe http https://google.com
./probe http -timeout 3s https://example.com
./probe http -loop 1m https://google.com         # Her 1 dakikada çalıştır

# TLS/SSL Strict Kontrol
./probe tls google.com:443
./probe tls -timeout 10s example.com:443
./probe tls -loop 30m google.com:443             # Her 30 dakikada çalıştır

# TLS/SSL Yalnızca Sertifika Kontrolü
./probe tls-cert google.com:443
./probe tls-cert https://google.com
./probe tls-cert -timeout 10s mtls.example.com:443
```

## Parametreler

```
-attempts int      Maksimum deneme sayısı (varsayılan: 3)
-threshold int     Geçmek için gereken başarılı deneme (varsayılan: 1)
-timeout duration  Deneme başına zaman aşımı (varsayılan: ping/tcp/http için 1s, tls/tls-cert için 5s)
-interval duration Denemeler arası bekleme (varsayılan: 500ms)
-loop duration     Loop aralığı (0 = bir kez çalıştır, örn: 5s, 1m, 10s)
-size int          ICMP payload boyutu, yalnızca ping (varsayılan: 56)
-debug             Deneme ayrıntısını stderr'e yaz
```

**Parametreler hedeften önce yazılmalı.** Go'nun flag ayrıştırıcısı ilk
parametre olmayan argümanda durur; hedeften sonra yazılan her şey aksi hâlde
sessizce yok sayılırdı. `probe` bunun yerine reddediyor:

```
$ probe tcp example.com:22 -attempts 5
Error: unexpected argument(s) after the target: -attempts 5
Flags must come before the target, e.g. probe tcp -attempts 5 example.com:22
```

## Komutlar

```
ping               ICMP ping probe'u (IP adresleri ve hostname'leri destekler)
tcp                TCP port bağlantısı kontrolü (host:port formatı)
http               HTTP/HTTPS durum kontrolü (URL formatı)
tls                Strict TLS/SSL handshake ve sertifika doğrulaması (host:port veya URL formatı)
tls-cert           Tam handshake gerektirmeden TLS/SSL sertifika incelemesi (host:port veya URL formatı)
```

## Zaman Aşımı Konfigürasyonu

- **ICMP Ping**: Deneme başına 1 saniye
- **TCP**: Deneme başına 1 saniye
- **HTTP/HTTPS**: Deneme başına 1 saniye
- **TLS/SSL**: Deneme başına 5 saniye

Her probe varsayılan olarak 3 yeniden deneme girişimini destekler.

## Docker Kullanımı

```bash
# Docker image'ını oluştur
docker build -t probe .

# Container'ı çalıştır
docker run --rm probe ping 8.8.8.8
docker run --rm probe tcp google.com:443
docker run --rm probe http https://google.com
docker run --rm probe tls google.com:443
docker run --rm probe tls-cert mtls.example.com:443

# Özel deneme sayısı ve timeout ile çalıştır
docker run --rm probe ping -attempts 5 8.8.8.8
docker run --rm probe tcp -timeout 2s google.com:443

# Loop modunda çalıştır
docker run --rm probe ping -loop 10s 8.8.8.8
```

### Docker Notu
Container, yetkisiz bir kullanıcı (`probeuser`) olarak çalışır.

## Çıktı Örnekleri

### ICMP Ping
```
Target: 8.8.8.8
Attempt: 1
Success: true
Duration: 45.32 ms
```

`Attempt`, gerçekte kaç deneme yapıldığını gösterir. Eşik birden büyükse ya da
başarısız bir deneme olduysa sayaçlar ve dağılım da raporlanır:

```
$ probe ping -attempts 5 -threshold 3 8.8.8.8
Target: 8.8.8.8
Attempt: 4
Success: true
Successes: 3  Failures: 1
Duration: 45.32 ms
Min/Avg/Max: 44.90 ms / 45.60 ms / 46.51 ms
```

### TCP Probe
```
Host: google.com
Port: 443
Attempt: 3
Success: true
Duration: 125.45 ms
Local Address: 192.168.1.100:52341
Remote Address: 142.251.32.14:443
```

### HTTP/HTTPS
```
URL: https://google.com
Attempt: 3
Success: true
Status Code: 200
Duration: 125.45 ms
```

### TLS/SSL (strict)
```
Host: google.com
Attempt: 1
Success: true
Duration: 235.67 ms
Subject: CN=www.google.com
Issuer: CN=Google Internet Authority G3
Valid From: 2026-08-01 00:00:00
Expires At: 2027-07-28 23:59:59
Days Until Expiry: 310
TLS Version: TLS 1.3
Cipher Suite: TLS_AES_128_GCM_SHA256
```

Sunucu, leaf'ten önce süresi dolan ara sertifikalar sunuyorsa en zayıf halka da
raporlanır — yalnızca leaf'e bakmanın gizlediği durum tam olarak budur:

```
Days Until Expiry: 310
Chain Expires At: 2026-10-01 12:00:00 (10 days, 2 certificates presented)
```

### TLS/SSL (yalnızca sertifika)
```
Host: mtls.example.com
Attempt: 1
Success: true
Duration: 210.12 ms
Subject: CN=mtls.example.com
Issuer: CN=Example Issuing CA
Valid From: 2026-09-21 00:00:00
Expires At: 2027-04-22 23:59:59
Days Until Expiry: 213
TLS Version: TLS 1.3
Cipher Suite: TLS_AES_128_GCM_SHA256
Handshake Warning: remote error: tls: certificate required
```

### Sertifika geçerliliği

Strict modda süresi dolmuş ya da henüz geçerli olmayan bir sertifikayı zaten
handshake'in kendisi reddeder. Yalnızca sertifika modu ise doğrulamayı tasarım
gereği atlar — mTLS isteyen bir sunucudan sertifikayı okuyabilmesinin yolu
budur — bu yüzden tarihler ayrıca kontrol edilir. Süresi dolmuş bir sertifika
başarısızlık olarak raporlanır, çünkü bir sağlık kontrolünün böyle bir
sertifika için "başarılı" demesi tam olarak yanlış cevaptır; ayrıntılar yine de
yazdırılır ki sorun görünür olsun:

```
$ probe tls-cert expired.example.com:443
Host: expired.example.com
Attempt: 1
Success: false
Successes: 0  Failures: 1
Subject: CN=expired.example.com
Valid From: 2025-08-16 23:17:20
Expires At: 2026-09-10 23:17:20
Days Until Expiry: -10
Certificate Status: EXPIRED
TLS Version: TLS 1.3
Cipher Suite: TLS_AES_128_GCM_SHA256
Error: certificate expired on 2026-09-10T23:17:20Z (10 days ago)
```

Aynısı geçerliliği henüz başlamamış sertifikalar (`Certificate Status: NOT YET
VALID`) ve sunulan zincirde süresi dolmuş herhangi bir sertifika için de
geçerlidir.

## Bağımlılıklar

| Paket | Ne için gerekli | Platform |
|---|---|---|
| `golang.org/x/net` (`icmp`, `ipv4`, `ipv6`) | ICMP ping | Yalnızca Linux |

Geri kalan her şey — TCP, HTTP ve TLS probe'ları — yalnızca Go standart
kütüphanesini kullanır. Windows'ta ICMP ping, Win32 `iphlpapi.dll` API'si
üzerinden uygulandığı için harici paket gerektirmez; diğer platformlarda ICMP
ping uygulanmamıştır. Dolayısıyla `golang.org/x/net` yalnızca Linux derlemesine
dahil olur.

`golang.org/x/sys`, `go.mod` içinde `golang.org/x/net`'in dolaylı bağımlılığı
olarak görünür.

## Gereksinimler

- Go 1.26 veya üstü (`go.mod` içinde belirtilen sürüm)
- Linux'ta ICMP Ping için:
  - Kernel desteğini kontrol edin: `cat /proc/sys/net/ipv4/ping_group_range`
  - Çekirdekte unprivileged ICMP etkinleştirilmiş olmalı
  - Etkinleştirmek için: `sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"` (systemd tabanlı çoğu dağıtımda zaten varsayılan; asıl container ve minimal imajlarda gerekir)
- TCP ve TLS probe'ları için internet bağlantısı

## Hata İşleme

Tüm probe'lar `-attempts` kadar (varsayılan 3) yeniden dener ve denemeler
arasında `-interval` kadar bekler (varsayılan 500 ms). Probe, `-threshold`
kadar deneme başarılı olur olmaz durur — varsayılan eşik 1 olduğu için bu ilk
başarı demektir — ve kalan denemeler eşiğe artık yetişemiyorsa yine durur,
çünkü devam etmenin öğreteceği bir şey kalmamıştır. Sorun giderme için
ayrıntılı hata mesajları üretilir.

### Çıkış kodları

| Mod | Davranış |
|---|---|
| Tek seferlik | Probe başarılıysa `0`, başarısızsa `1` |
| Döngü (`-loop`) | Başarısız tur raporlanır ve döngü devam eder; süreç yalnızca `Ctrl+C` ile biter |

`Ctrl+C` yalnızca süreci öldürmekle kalmaz, uçuştaki probe'u da iptal eder;
döngü modunda bir sonraki tick'i beklemeden çıkar. İkinci `Ctrl+C` süreci
anında sonlandırır.

Geçersiz argümanlar ilk probe çalışmadan önce, `1` çıkış koduyla reddedilir:
`-attempts` ve `-threshold` en az 1 olmalı, `-threshold` `-attempts`'i aşmamalı,
`-timeout`, `-interval`, `-loop` ve `-size` negatif olmamalı, bilinmeyen komut
her turda değil bir kez raporlanır, hedeften sonra gelen her argüman
reddedilir.

### Gidiş-dönüş süresi nasıl ölçülür

| Platform | Kaynak |
|---|---|
| Linux | `time.Since`; sayaç gönderimden hemen önce başlatılır, cevap gelir gelmez okunur. Go'nun monotonic saatini kullanır; bu sayede duvar saati değişiklikleri — NTP sıçramaları, elle değişiklik, VM geri yükleme — sonucu bozamaz ya da negatife çeviremez. Ölçüm süreç içinde yapıldığı için gönderim ve alım sistem çağrılarının maliyetini de içerir. |
| Windows | `ICMP_ECHO_REPLY.RoundTripTime`; Win32 ICMP API'sinin kendi bildirdiği değer. Çözünürlüğü 1 ms'dir; daha hızlı cevaplarda 0 döner ve o durumda yerel olarak ölçülen süre kullanılır. |

İkisi birbirine yakındır ama birebir aynı değildir, çünkü farklı noktalardan
alınır: Linux'taki değer sistem çağrılarının etrafında ölçülür, Windows'taki
işletim sistemi tarafından üretilir. Platformlar arası karşılaştırmayı
mikrosaniye düzeyinde değil, büyüklük düzeyinde yapın.
