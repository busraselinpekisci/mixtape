#!/bin/bash

rm -rf JPEGImages/*out*
for INPUT in $(ls JPEGImages | xargs)
do
   python3 object_detection_yolo.py --image JPEGImages/$INPUT  --weights $1 --nmst 0.1 --conft 0.5
done

