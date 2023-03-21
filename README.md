# nft-backend

## 上传接口，可以同时上传到IPFS
合成了boostx的子命令，http接口上传文件到IPFS
https://boost.filecoin.io/tutorials/how-to-store-files-with-boost-on-filecoin
```text
/upload 
```
```json
{
"cid": "Qmb4LUTK8KiH5MBhiUkoWzc2zkTxxpzjbCDSsdufmVtP8Y",
"link": "http://127.0.0.1:8080/ipfs/Qmb4LUTK8KiH5MBhiUkoWzc2zkTxxpzjbCDSsdufmVtP8Y",
"car_link": "http://127.0.0.1:8080/ipfs/ipfs/QmS63ZsRbh3239SJYKyumFu7x4V6kRDNZxrjGcxQwXi4xV",
"payload_cid": "bafk2bzacecryctcw46nurofuhejp2h4f5rmb6lk7pal7wfcxcjmgeaxx3xhrw",
"comm_p_cid": "baga6ea4seaqjzvh4v3ntvjn4il2tyneabk7h3ydjplaoxqwsuqxwuxk5cplvsnq",
"piece_size": 1024,
"car_file_size": 667
}
```
