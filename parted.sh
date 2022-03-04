#!/bin/bash  
PATH=/bin:/sbin:/usr/bin:/usr/sbin
export PATH 
disk_to_parted="$1"
type="$2"
start="$3" 
end="$4"
name="$5"
file="part.txt"

if  [[ -z "$disk_to_parted" ]];then
    echo "no disk to parted,example $disk_to_parted"
    exit
elif [[ -z "$type" ]];then
    echo "no filesystem type set  ${type},example ext4/xfs"
    exit
elif  [[ -z "$start" ]];then
    echo "no partition location:start,example 0"
    exit 
elif  [[ -z "$end" ]];then
     "no partition location:end,example -1"   
elif  [[ -z "$name" ]];then
     "no partition name,example testloop"   
fi
echo "Input Param: $disk_to_parted ${type} ${start} ${end}"
rm -rf $file && touch $file && chmod a+x $file
echo "mklabel gpt" >> $file
echo "yes" >> $file
echo "p" >> $file
echo "mkpart ${name} ${type}  ${start}  ${end}" >> $file
echo "ignore" >> $file
echo "p" >> $file
echo "quit"  >> $file
/sbin/parted  ${disk_to_parted} < ./part.txt
sleep 1s
/sbin/blkid ${disk_to_parted}
lsblk ${disk_to_parted}
fdisk -lu ${disk_to_parted}