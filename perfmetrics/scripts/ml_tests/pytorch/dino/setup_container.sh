#!/bin/bash

# Install golang
wget -O go_tar.tar.gz https://go.dev/dl/go1.21.0.linux-amd64.tar.gz -q
rm -rf /usr/local/go && tar -C /usr/local -xzf go_tar.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Clone and build the gcsfuse master branch.
git clone https://github.com/GoogleCloudPlatform/gcsfuse.git
cd gcsfuse
CGO_ENABLED=0 go build .
cd -

# Create a directory for gcsfuse logs
mkdir  run_artifacts/gcsfuse_logs

echo "Mounting GCSFuse..."
nohup /pytorch_dino/gcsfuse/gcsfuse --foreground --type-cache-ttl=1728000s \
        --stat-cache-ttl=1728000s \
        --stat-cache-capacity=1320000 \
        --stackdriver-export-interval=60s \
        --implicit-dirs \
        --max-conns-per-host=100 \
        --debug_fuse \
        --debug_gcs \
        --log-file run_artifacts/gcsfuse.log \
        --log-format text \
       gcsfuse-ml-data gcsfuse_data > "run_artifacts/gcsfuse.out" 2> "run_artifacts/gcsfuse.err" &

# Update the pytorch library code to bypass the kernel-cache
echo "Updating the pytorch library code to bypass the kernel-cache..."
echo "
def pil_loader(path: str) -> Image.Image:
    fd = os.open(path, os.O_DIRECT)
    f = os.fdopen(fd, \"rb\")
    img = Image.open(f)
    rgb_img = img.convert(\"RGB\")
    f.close()
    return rgb_img
" > bypassed_code.py

folder_file="/opt/conda/lib/python3.7/site-packages/torchvision/datasets/folder.py"
x=$(grep -n "def pil_loader(path: str) -> Image.Image:" $folder_file | cut -f1 -d ':')
y=$(grep -n "def accimage_loader(path: str) -> Any:" $folder_file | cut -f1 -d ':')
y=$((y - 2))
lines="$x,$y"
sed -i "$lines"'d' $folder_file
sed -i "$x"'r bypassed_code.py' $folder_file

# Fix the caching issue - comes when we run the model first time with 8
# nproc_per_node - by downloading the model in single thread environment.
python -c 'import torch;torch.hub.list("facebookresearch/xcit:main")'

ARTIFACTS_BUCKET_PATH="gs://gcsfuse-ml-tests-logs/ci_artifacts/pytorch/dino"
echo "Update status file"
echo "RUNNING" > status.txt
gsutil cp status.txt $ARTIFACTS_BUCKET_PATH/

echo "Update start time file"
echo $(date +"%s") > start_time.txt
gsutil cp start_time.txt $ARTIFACTS_BUCKET_PATH/

(
  set +e
  # Run the pytorch Dino model
  # We need to run it in foreground mode to make the container running.
  echo "Running the pytorch dino model..."
  experiment=dino_experiment
  python3 -m torch.distributed.launch \
    --nproc_per_node=2 dino/main_dino.py \
    --arch vit_small \
    --num_workers 20 \
    --data_path gcsfuse_data/imagenet/ILSVRC/Data/CLS-LOC/train/ \
    --output_dir "./run_artifacts/$experiment" \
    --norm_last_layer False \
    --use_fp16 False \
    --clip_grad 0 \
    --epochs 80 \
    --global_crops_scale 0.25 1.0 \
    --local_crops_number 10 \
    --local_crops_scale 0.05 0.25 \
    --teacher_temp 0.07 \
    --warmup_teacher_temp_epochs 30 \
    --clip_grad 0 \
    --min_lr 0.00001
    if [ $? -eq 0 ];
    then
        echo "Pytorch dino model completed the training successfully!"
        echo "COMPLETE" > status.txt
    else
        echo "Pytorch dino model training failed!"
        echo "ERROR" > status.txt
    fi
)

gsutil cp status.txt $ARTIFACTS_BUCKET_PATH/
