package mp4

import (
    "encoding/binary"
    "errors"
    "io"

    "github.com/yapingcat/gomedia/mpeg"
)

type AVPacket struct {
    Cid     MP4_CODEC_TYPE
    Data    []byte
    TrackId int
    Pts     uint64
    Dts     uint64
}

type TrackInfo struct {
    Duration     uint32
    TrackId      int
    Cid          MP4_CODEC_TYPE
    Height       uint32
    Width        uint32
    SampleRate   uint32
    SampleSize   uint16
    ChannelCount uint8
    Timescale    uint32
}

type Mp4Info struct {
    MajorBrand       uint32
    MinorVersion     uint32
    CompatibleBrands []uint32
    Duration         uint32
    Timescale        uint32
    CreateTime       uint64
    ModifyTime       uint64
}

type MovDemuxer struct {
    readerHandler Reader
    mdatOffset    []uint64 //一个mp4文件可能存在多个mdatbox
    tracks        []*mp4track
    readSampleIdx []uint32
    mp4out        []byte
    mp4Info       Mp4Info
}

// how to demux mp4 file
// 1. CreateMovDemuxer
// 2. ReadHead()
// 3. ReadPacket

func CreateMp4Demuxer(rh Reader) *MovDemuxer {
    return &MovDemuxer{
        readerHandler: rh,
    }
}

func (demuxer *MovDemuxer) ReadHead() ([]TrackInfo, error) {
    infos := make([]TrackInfo, 0, 2)
    var err error
    for {
        fullbox := FullBox{}
        basebox := BasicBox{}
        _, err = basebox.Decode(demuxer.readerHandler)
        if err != nil {
            break
        }
        if basebox.Size < BasicBoxLen {
            err = errors.New("mp4 Parser error")
            break
        }
        switch mov_tag(basebox.Type) {
        case mov_tag([4]byte{'f', 't', 'y', 'p'}):
            err = decodeFtypBox(demuxer, uint32(basebox.Size))
        case mov_tag([4]byte{'f', 'r', 'e', 'e'}):
            err = decodeFreeBox(demuxer)
        case mov_tag([4]byte{'m', 'd', 'a', 't'}):
            demuxer.mdatOffset = append(demuxer.mdatOffset, uint64(demuxer.readerHandler.Tell()))
            _, err = demuxer.readerHandler.Seek(int64(basebox.Size)-BasicBoxLen, 1)
        case mov_tag([4]byte{'m', 'o', 'o', 'v'}):
            currentOffset := demuxer.readerHandler.Tell()
            offset := int64(0)
            if offset, err = demuxer.readerHandler.Seek(0, io.SeekEnd); err != nil {
                break
            }
            if offset-currentOffset < int64(basebox.Size)-BasicBoxLen {
                err = errors.New("incomplete mp4 file")
                break
            }
            _, err = demuxer.readerHandler.Seek(currentOffset, io.SeekStart)
        case mov_tag([4]byte{'m', 'v', 'h', 'd'}):
            err = decodeMvhd(demuxer)
        case mov_tag([4]byte{'t', 'r', 'a', 'k'}):
            track := &mp4track{}
            demuxer.tracks = append(demuxer.tracks, track)
        case mov_tag([4]byte{'t', 'k', 'h', 'd'}):
            err = decodeTkhdBox(demuxer)
        case mov_tag([4]byte{'m', 'd', 'h', 'd'}):
            err = decodeMdhdBox(demuxer)
        case mov_tag([4]byte{'h', 'd', 'l', 'r'}):
            err = decodeHdlrBox(demuxer, basebox.Size)
        case mov_tag([4]byte{'m', 'd', 'i', 'a'}):
        case mov_tag([4]byte{'m', 'i', 'n', 'f'}):
        case mov_tag([4]byte{'v', 'm', 'h', 'd'}):
            err = decodeVmhdBox(demuxer)
        case mov_tag([4]byte{'s', 'm', 'h', 'd'}):
            err = decodeSmhdBox(demuxer)
        case mov_tag([4]byte{'h', 'm', 'h', 'd'}):
            _, err = fullbox.Decode(demuxer.readerHandler)
        case mov_tag([4]byte{'n', 'm', 'h', 'd'}):
            _, err = fullbox.Decode(demuxer.readerHandler)
        case mov_tag([4]byte{'s', 't', 'b', 'l'}):
            demuxer.tracks[len(demuxer.tracks)-1].stbltable = new(movstbl)
        case mov_tag([4]byte{'s', 't', 's', 'd'}):
            err = decodeStsdBox(demuxer)
        case mov_tag([4]byte{'s', 't', 't', 's'}):
            err = decodeSttsBox(demuxer)
        case mov_tag([4]byte{'c', 't', 't', 's'}):
            err = decodeCttsBox(demuxer)
        case mov_tag([4]byte{'s', 't', 's', 'c'}):
            err = decodeStscBox(demuxer)
        case mov_tag([4]byte{'s', 't', 's', 'z'}):
            err = decodeStszBox(demuxer)
        case mov_tag([4]byte{'s', 't', 'c', 'o'}):
            err = decodeStcoBox(demuxer)
        case mov_tag([4]byte{'c', 'o', '6', '4'}):
            err = decodeCo64Box(demuxer)
        case mov_tag([4]byte{'a', 'v', 'c', '1'}):
            demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_H264
            demuxer.tracks[len(demuxer.tracks)-1].extra = new(h264ExtraData)
            err = decodeVisualSampleEntry(demuxer)
        case mov_tag([4]byte{'h', 'v', 'c', '1'}):
            demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_H265
            demuxer.tracks[len(demuxer.tracks)-1].extra = newh265ExtraData()
            err = decodeVisualSampleEntry(demuxer)
        case mov_tag([4]byte{'m', 'p', '4', 'a'}):
            demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_AAC
            demuxer.tracks[len(demuxer.tracks)-1].extra = new(aacExtraData)
            err = decodeAudioSampleEntry(demuxer)
        case mov_tag([4]byte{'u', 'l', 'a', 'w'}):
            demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_G711U
            err = decodeAudioSampleEntry(demuxer)
        case mov_tag([4]byte{'a', 'l', 'a', 'w'}):
            demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_G711A
            err = decodeAudioSampleEntry(demuxer)
        case mov_tag([4]byte{'a', 'v', 'c', 'C'}):
            err = decodeAvccBox(demuxer, uint32(basebox.Size))
        case mov_tag([4]byte{'h', 'v', 'c', 'C'}):
            err = decodeHvccBox(demuxer, uint32(basebox.Size))
        case mov_tag([4]byte{'e', 's', 'd', 's'}):
            err = decodeEsdsBox(demuxer, uint32(basebox.Size))
        //case mov_tag([4]byte{'e', 'd', 't', 's'}):
        //case mov_tag([4]byte{'m', 'v', 'e', 'x'}):
        default:
            //panic(1)
            _, err = demuxer.readerHandler.Seek(int64(basebox.Size)-BasicBoxLen, io.SeekCurrent)
        }
        if err != nil {
            break
        }
    }
    if err != nil && err != io.EOF {
        return nil, err
    }
    demuxer.buildSampleList()
    for _, track := range demuxer.tracks {
        info := TrackInfo{}
        info.Cid = track.cid
        info.Duration = track.duration
        info.ChannelCount = track.chanelCount
        info.SampleRate = track.sampleRate
        info.SampleSize = uint16(track.sampleBits)
        info.TrackId = int(track.trackId)
        info.Width = track.width
        info.Height = track.height
        info.Timescale = track.timescale
        infos = append(infos, info)

    }
    return infos, nil
}

func (demuxer *MovDemuxer) GetMp4Info() Mp4Info {
    return demuxer.mp4Info
}

///return error == io.EOF, means read mp4 file completed
func (demuxer *MovDemuxer) ReadPacket() (*AVPacket, error) {
    if len(demuxer.readSampleIdx) == 0 {
        demuxer.readSampleIdx = make([]uint32, len(demuxer.tracks))
    }
    for {
        maxdts := int64(-1)
        minTsSample := sampleEntry{dts: uint64(maxdts)}
        var whichTrack *mp4track = nil
        whichTracki := 0
        for i, track := range demuxer.tracks {
            idx := demuxer.readSampleIdx[i]
            if int(idx) == len(track.samplelist) {
                continue
            }
            if whichTrack == nil {
                minTsSample = track.samplelist[idx]
                whichTrack = track
                whichTracki = i
            } else {
                dts1 := minTsSample.dts * uint64(demuxer.mp4Info.Timescale) / uint64(whichTrack.timescale)
                dts2 := track.samplelist[idx].dts * uint64(demuxer.mp4Info.Timescale) / uint64(track.timescale)
                if dts1 > dts2 {
                    minTsSample = track.samplelist[idx]
                    whichTrack = track
                    whichTracki = i
                }
            }
        }

        if minTsSample.dts == uint64(maxdts) {
            return nil, io.EOF
        }
        if _, err := demuxer.readerHandler.Seek(int64(minTsSample.offset), io.SeekStart); err != nil {
            return nil, err
        }
        sample := make([]byte, minTsSample.size)
        if _, err := demuxer.readerHandler.ReadAtLeast(sample); err != nil {
            return nil, err
        }
        demuxer.readSampleIdx[whichTracki]++
        avpkg := &AVPacket{
            Cid:     whichTrack.cid,
            TrackId: int(whichTrack.trackId),
            Pts:     minTsSample.pts * uint64(demuxer.mp4Info.Timescale) / uint64(whichTrack.timescale),
            Dts:     minTsSample.dts * uint64(demuxer.mp4Info.Timescale) / uint64(whichTrack.timescale),
        }
        if whichTrack.cid == MP4_CODEC_H264 {
            extra, ok := whichTrack.extra.(*h264ExtraData)
            if !ok {
                panic("must init aacExtraData first")
            }
            avpkg.Data = demuxer.processH264(sample, extra)
        } else if whichTrack.cid == MP4_CODEC_H265 {
            extra, ok := whichTrack.extra.(*h265ExtraData)
            if !ok {
                panic("must init aacExtraData first")
            }
            avpkg.Data = demuxer.processH265(sample, extra)
        } else if whichTrack.cid == MP4_CODEC_AAC {
            aacExtra, ok := whichTrack.extra.(*aacExtraData)
            if !ok {
                panic("must init aacExtraData first")
            }
            aacframe := mpeg.ConvertASCToADTS(aacExtra.asc, len(sample)+7)
            aacframe = append(aacframe, sample...)
            avpkg.Data = aacframe
        } else {
            avpkg.Data = sample
        }
        if len(avpkg.Data) > 0 {
            return avpkg, nil
        }
    }
}

func (demuxer *MovDemuxer) buildSampleList() {
    for _, track := range demuxer.tracks {
        stbl := track.stbltable
        chunks := make([]movchunk, stbl.stco.entryCount)
        iterator := 0
        for i := 0; i < int(stbl.stco.entryCount); i++ {
            chunks[i].chunknum = uint32(i + 1)
            chunks[i].chunkoffset = stbl.stco.chunkOffsetlist[i]
            for iterator+1 < int(stbl.stsc.entryCount) && stbl.stsc.entrys[iterator+1].firstChunk <= chunks[i].chunknum {
                iterator++
            }
            chunks[i].samplenum = stbl.stsc.entrys[iterator].samplesPerChunk
        }
        track.samplelist = make([]sampleEntry, stbl.stsz.sampleCount)
        for i := range track.samplelist {
            if stbl.stsz.sampleSize == 0 {
                track.samplelist[i].size = uint64(stbl.stsz.entrySizelist[i])
            } else {
                track.samplelist[i].size = uint64(stbl.stsz.sampleSize)
            }
        }
        iterator = 0
        for i := range chunks {
            track.samplelist[iterator].offset = chunks[i].chunkoffset
            iterator++
            for j := 1; j < int(chunks[i].samplenum); j++ {
                track.samplelist[iterator].offset = track.samplelist[iterator-1].offset + track.samplelist[iterator-1].size
                iterator++
            }
        }
        iterator = 0
        track.samplelist[iterator].dts = 0
        iterator++
        for i := range stbl.stts.entrys {
            for j := 0; j < int(stbl.stts.entrys[i].sampleCount); j++ {
                if iterator == len(track.samplelist) {
                    break
                }
                track.samplelist[iterator].dts = uint64(stbl.stts.entrys[i].sampleDelta) + track.samplelist[iterator-1].dts
                iterator++
            }
        }

        // no ctts table, so pts == dts
        if stbl.ctts == nil || stbl.ctts.entryCount == 0 {
            for i := range track.samplelist {
                track.samplelist[i].pts = track.samplelist[i].dts
            }
        } else {
            iterator = 0
            for i := range stbl.ctts.entrys {
                for j := 0; j < int(stbl.ctts.entrys[i].sampleCount); j++ {
                    track.samplelist[iterator].pts = track.samplelist[iterator].dts + uint64(stbl.ctts.entrys[i].sampleOffset)
                    iterator++
                }
            }
        }
    }

}

func (demuxer *MovDemuxer) processH264(avcc []byte, extra *h264ExtraData) []byte {
    idr := false
    vcl := false
    spspps := false
    h264 := avcc
    for len(h264) > 0 {
        nalusize := binary.BigEndian.Uint32(h264)
        mpeg.CovertAVCCToAnnexB(h264)
        nalType := mpeg.H264NaluType(h264)
        switch {
        case nalType == mpeg.H264_NAL_PPS:
            fallthrough
        case nalType == mpeg.H264_NAL_SPS:
            spspps = true
        case nalType == mpeg.H264_NAL_I_SLICE:
            idr = true
            fallthrough
        case nalType >= mpeg.H264_NAL_P_SLICE && nalType <= mpeg.H264_NAL_SLICE_C:
            vcl = true
        }
        h264 = h264[4+nalusize:]
    }

    if !vcl {
        if !spspps {
            return avcc
        } else {
            demuxer.mp4out = append(demuxer.mp4out, avcc...)
        }
        return nil
    }

    if spspps {
        demuxer.mp4out = demuxer.mp4out[:0]
        return avcc
    }
    if !idr {
        return avcc
    }
    if len(demuxer.mp4out) > 0 {
        out := make([]byte, len(demuxer.mp4out)+len(avcc))
        copy(out, demuxer.mp4out)
        copy(out[len(demuxer.mp4out):], avcc)
        demuxer.mp4out = demuxer.mp4out[:0]
        return out
    }

    out := make([]byte, 0)
    for _, sps := range extra.spss {
        out = append(out, sps...)
    }
    for _, pps := range extra.ppss {
        out = append(out, pps...)
    }
    out = append(out, avcc...)
    return out
}

func (demuxer *MovDemuxer) processH265(hvcc []byte, extra *h265ExtraData) []byte {
    idr := false
    vcl := false
    spsppsvps := false
    h265 := hvcc
    for len(hvcc) > 0 {
        nalusize := binary.BigEndian.Uint32(h265)
        mpeg.CovertAVCCToAnnexB(h265)
        nalType := mpeg.H265NaluType(h265)
        switch {
        case nalType == mpeg.H265_NAL_VPS:
            fallthrough
        case nalType == mpeg.H265_NAL_PPS:
            fallthrough
        case nalType == mpeg.H265_NAL_SPS:
            spsppsvps = true
        case nalType >= mpeg.H265_NAL_SLICE_BLA_W_LP && nalType <= mpeg.H265_NAL_SLICE_CRA:
            idr = true
            fallthrough
        case nalType >= mpeg.H265_NAL_Slice_TRAIL_N && nalType <= mpeg.H265_NAL_SLICE_RASL_R:
            vcl = true
        }
        h265 = h265[4+nalusize:]
    }
    if !vcl {
        if !spsppsvps {
            return hvcc
        } else {
            demuxer.mp4out = append(demuxer.mp4out, hvcc...)
        }
        return nil
    }

    if spsppsvps {
        demuxer.mp4out = demuxer.mp4out[:0]
        return hvcc
    }
    if !idr {
        return hvcc
    }
    if len(demuxer.mp4out) > 0 {
        out := make([]byte, len(demuxer.mp4out)+len(hvcc))
        copy(out, demuxer.mp4out)
        copy(out[len(demuxer.mp4out):], hvcc)
        demuxer.mp4out = demuxer.mp4out[:0]
        return out
    }

    out := extra.hvccExtra.ToNalus()
    out = append(out, hvcc...)
    return out
}