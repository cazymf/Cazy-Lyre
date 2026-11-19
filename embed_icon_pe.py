"""Embed ICO as a real Win32 RT_ICON/RT_GROUP_ICON resource into Go-built PE.
Requires Python 3.11 and GNU objcopy (for build-time use only). No third-party packages.
Can also be run after building the EXE. Does not alter code/data sections.
"""
import argparse
import os
import struct
import subprocess
import tempfile
from pathlib import Path


def u16(b, o): return struct.unpack_from('<H', b, o)[0]
def u32(b, o): return struct.unpack_from('<I', b, o)[0]

def read_ico(path):
    data = Path(path).read_bytes()
    reserved, kind, count = struct.unpack_from('<HHH', data, 0)
    if reserved != 0 or kind != 1 or not (1 <= count <= 32):
        raise ValueError('Not a supported ICO (must contain 1-32 icons)')
    entries = []
    for i in range(count):
        off = 6 + i*16
        width, height, colors, reserved, planes, bitcount, size, img_off = struct.unpack_from('<BBBBHHII', data, off)
        if img_off + size > len(data):
            raise ValueError('Malformed ICO image offset')
        entries.append((width,height,colors,reserved,planes,bitcount,data[img_off:img_off+size]))
    return entries

class ResBuilder:
    def __init__(self):
        self.data=bytearray()
        self.patch_rvas=[]
    def add(self, b, align=4):
        while len(self.data)%align:self.data.append(0)
        start=len(self.data)
        self.data += b
        return start
    def directory(self, n):
        return self.add(struct.pack('<IIHHHH',0,0,0,0,0,n)+bytes(n*8))
    def link(self, directory, i, resource_id, off, subdir=True):
        struct.pack_into('<II',self.data,directory+16+i*8,resource_id,off|(0x80000000 if subdir else 0))
    def resource(self, parent, i, res_id, payload):
        lang=self.directory(1)
        self.link(parent,i,res_id,lang)
        leaf=self.add(bytes(16))
        self.link(lang,0,1033,leaf,False)
        chunk=self.add(payload)
        struct.pack_into('<IIII',self.data,leaf,chunk,len(payload),0,0)
        self.patch_rvas.append(leaf)


def make_rsrc(ico_path):
    images=read_ico(ico_path)
    b=ResBuilder()
    root=b.directory(2)
    icons_type=b.directory(len(images))
    grp_type=b.directory(1)
    b.link(root,0,3,icons_type)
    b.link(root,1,14,grp_type)
    grp=bytearray(struct.pack('<HHH',0,1,len(images)))
    for i,(width,height,colors,reserved,planes,bitcount,image) in enumerate(images):
        b.resource(icons_type,i,i+1,image)
        grp+=struct.pack('<BBBBHHIH',width,height,colors,reserved,planes,bitcount,len(image),i+1)
    b.resource(grp_type,0,1,grp)
    return b.data,b.patch_rvas


def pe_section_info(buf, section_name):
    pe=u32(buf,0x3c)
    if buf[pe:pe+4]!=b'PE\0\0':raise ValueError('Not a PE executable')
    filehdr=pe+4
    n=u16(buf,filehdr+2)
    size_opt=u16(buf,filehdr+16)
    opt=filehdr+20
    magic=u16(buf,opt)
    if magic!=0x20b:raise ValueError('Only PE32+ x64 supported')
    table=opt+size_opt
    for i in range(n):
        at=table+i*40
        name=buf[at:at+8].split(b'\0',1)[0].decode('ascii')
        if name==section_name:
            return {'rva':u32(buf,at+12),'raw_size':u32(buf,at+16),'raw_offset':u32(buf,at+20),'virtual_size':u32(buf,at+8),'dir_offset':opt+112+8*2}
    raise ValueError('Resource section not found after objcopy')


def embed(exe, ico, objcopy='objcopy'):
    exe=Path(exe)
    resource,patches=make_rsrc(ico)
    with tempfile.TemporaryDirectory() as temp:
        raw=Path(temp)/'icon.rsrc'
        raw.write_bytes(resource)
        new=Path(temp)/'with_icon.exe'
        original=exe.read_bytes()
        pe=u32(original,0x3c)
        opt=pe+24
        image_base=struct.unpack_from('<Q',original,opt+24)[0]
        image_size=u32(original,opt+56)
        aligned_rva=(image_size+0xFFF)&~0xFFF
        absolute_vma=image_base+aligned_rva
        subprocess.run([objcopy,'--add-section',f'.rsrc={raw}',
                        '--set-section-flags','.rsrc=alloc,load,readonly,data',
                        '--change-section-vma',f'.rsrc=0x{absolute_vma:X}',
                        str(exe),str(new)],check=True)
        out=bytearray(new.read_bytes())
        info=pe_section_info(out,'.rsrc')
        if len(resource)>info['raw_size']:raise ValueError('Resource data exceeds section')
        section_offset=info['raw_offset']; section_rva=info['rva']
        for leaf in patches:
            original_offset=u32(out,section_offset+leaf)
            struct.pack_into('<I',out,section_offset+leaf,section_rva+original_offset)
        struct.pack_into('<II',out,info['dir_offset'],section_rva,len(resource))
        exe.write_bytes(out)
        if info['rva']==0 or u32(out,info['dir_offset'])==0:
            raise ValueError('Resource directory not installed')
        print(f'EXE icon embedded: {len(patches)-1} icon images; resource RVA=0x{section_rva:X}, size={len(resource)}')

if __name__=='__main__':
    ap=argparse.ArgumentParser()
    ap.add_argument('exe');ap.add_argument('icon');ap.add_argument('--objcopy',default='objcopy')
    args=ap.parse_args()
    embed(args.exe,args.icon,args.objcopy)
