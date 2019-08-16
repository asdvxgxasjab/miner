## To build

```bash
cd opencl
mkdir build
cd build
cmake ..
make
sudo make install
cd ../../client
go build
cp ../opencl/cruzbit.cl .
LD_LIBRARY_PATH=/usr/local/lib ./miner -pubkey <your public key> -peer <pool address>
```

### This is not very polished at the moment. Use at your own risk!
